package main

import (
	"strings"
	"context"
	"flag"
	"fmt"
	"github.com/op/go-logging"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/cheggaaa/pb/v3"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"time"
)

var (
	region      string
	bucket      string
	prefix      string
	writeDir    string
	downloaders uint
	threads     uint
	partSize    int64
	maxListKeys int

	startTimeNanos int64
	benchmark      bool
	loglevel       string
	cpuprofile     string
	log = logging.MustGetLogger("s3pd")
	downloadBar = pb.New(1)
)

func init() {
	flag.StringVar(&region, "region", "", "AWS Region")
	flag.StringVar(&bucket, "bucket", "", "S3 bucket to download from")
	flag.StringVar(&prefix, "prefix", "", "S3 prefix of objects (Optional)")
	flag.StringVar(&writeDir, "writedir", "", "Directory to write files to")
	flag.UintVar(&downloaders, "downloaders", 10, "Number of concurrent s3 downloads")
	flag.UintVar(&threads, "threads", 5, "Number of threads used by each downloader")
	flag.Int64Var(&partSize, "partsize", 5*1024*1024, "bytes to assign each thread to download, default of 5*1024*1024 (5MiB)")
	flag.IntVar(&maxListKeys, "maxlistkeys", 1000, "max number of object keys to return in each ls pagination request")
	flag.BoolVar(&benchmark, "benchmark", false, "set to true to test raw download to ram speed")
	flag.StringVar(&loglevel, "loglevel", "NOTICE", "Level of logging to expose, INFO, NOTICE, WARNING, ERROR - Default is NOTICE")
	flag.StringVar(&cpuprofile, "cpuprofile", "", "Writes cpu profile to specified filepath")
	flag.Parse()
}

func worker(id int, downloader *s3manager.Downloader, jobs <-chan s3types.Object, wg *sync.WaitGroup) {
	defer wg.Done()

	for j := range jobs {
		path := fmt.Sprintf("%s/%s", writeDir, *j.Key)
		
		log.Debugf("worker-%d writing s3://%s/%s to %s [%.2fMiB]\n",
			id,
			bucket, *j.Key,
			path,
			(float64(j.Size) / 1024 / 1024))

		var w io.WriterAt
		if benchmark {
			w = NewDiscardWriteBuffer()
		} else {
			// ensure dir is created. MkdirAll returns nil if folder already exists
			if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
				panic(err)
			}

			var err error
			w, err = os.Create(path)
			if err != nil {
				panic(err)
			}
		}

		w = NewLogProgressWriteBuffer(downloadBar, w)

		_, err := downloader.Download(context.Background(), w, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    j.Key,
		})

		if err != nil {
			panic(err)
		}
	}
}

func list(client *s3.Client, jobs chan<- s3types.Object) error {
	log.Debugf("Listing objects with the prefix of s3://%s/%s\n", bucket, prefix)
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket:  &bucket,
		Prefix:  &prefix,
		MaxKeys: int32(maxListKeys),
	})


	var numBytes int64 = 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return fmt.Errorf("Error listing objects: %w", err)
		}
		if startTimeNanos == 0 {
			startTimeNanos = time.Now().UnixNano()
		}

		log.Debugf("Scheduling %d objects to be downloaded\n", len(page.Contents))

		for _, item := range page.Contents {
			jobs <- item
			numBytes += item.Size // size in Bytes
		}
		downloadBar.SetTotal(numBytes)
	}
	return nil
}

func main() {
	// Create new go-logging instance that sets logs to the declared level.
	lm := logging.AddModuleLevel(logging.NewLogBackend(os.Stderr, "", 0))
	logLevels := map[string]logging.Level{
		"DEBUG": logging.DEBUG,
		"INFO":   logging.INFO,
		"NOTICE": logging.NOTICE,
		"WARNING": logging.WARNING,
		"ERROR": logging.ERROR,
		"CRITICAL": logging.CRITICAL,
	}
	lm.SetLevel(logLevels[strings.ToUpper(loglevel)], "")
	logging.SetBackend(lm)

	// Check for manditory parameters
	if len(region) == 0 {
		fmt.Fprintln(os.Stderr, "missing --region")
		flag.PrintDefaults()
		return
	}
	if len(bucket) == 0 {
		fmt.Fprintln(os.Stderr, "missing --bucket")
		flag.PrintDefaults()
		return
	}
	if !benchmark && len(writeDir) == 0 {
		fmt.Fprintln(os.Stderr, "missing --writeDir")
		flag.PrintDefaults()
		return
	}

	// Let user know if benchmark mode is enabled
	// NOTE: When in benchmark mode this avoids needing to do filesystem IO in creating missing directories
	// and opening file for writing.
	// This is a savings of roughly 5 to 10ms per Object download request
	if benchmark {
		fmt.Println("Benchmark mode, data being written to temporary in memory object")
	}

	// If cpu profiling flag set, enable pprof cpu profiling - this will have a minor performance hit
	// every few hundred cpu cycles a snapshot of the program state will be taken
	if len(cpuprofile) != 0 {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	downloadStart := time.Now()

	// Create s3 client
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		fmt.Errorf("configuration error, %w", err)
		return
	}
	client := s3.NewFromConfig(cfg)

	// Instantiate download workers
	// Set channel length to 3x max objects we'll get in a list op
	// if the job queue ends up filling up, we'll stall doing additional list ops until the queue has more messages completed
	jobs := make(chan s3types.Object, maxListKeys * 3)
	wg := new(sync.WaitGroup)
	downloader := s3manager.NewDownloader(client, func(d *s3manager.Downloader) {
		d.PartSize = partSize
		d.Concurrency = int(threads)
		d.BufferProvider = s3manager.NewPooledBufferedWriterReadFromProvider(int(partSize))
	})
	
	for d := 1; d < int(downloaders); d++ {
		wg.Add(1)
		go worker(int(d), downloader, jobs, wg)
	}

	// Queue up download tasks
	if err := list(client, jobs); err != nil {
		panic(err)
	}

	// Indicate that we listed every single object and there's no more objs needing to be queued
	close(jobs)

	// Display the progress bar
	downloadBar.SetWriter(os.Stdout)
	downloadBar.Set(pb.SIBytesPrefix, false)
	downloadBar.Set(pb.Bytes, true)
	downloadBar.Start()

	// Wait till all downloads finish
	wg.Wait()
	downloadBar.Finish()

	// Print final throughput estimate
	fmt.Printf("\nAverage throughput was: %0.4fGibps\n",
		float64(downloadBar.Total()) * 8 / 1024 / 1024 / 1024 / time.Since(downloadStart).Seconds())
}

