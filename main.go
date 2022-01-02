package main

import (
	"context"
	"errors"
	flag "github.com/spf13/pflag"
	"fmt"
	"github.com/cheggaaa/pb/v3"
	"github.com/op/go-logging"
	"net/url"
	"os"
	"runtime/pprof"
	"strings"
)

var (
	// flags & args
	source      string
	destination string
	region      string
	downloaders uint
	threads     uint
	partsize    int64
	maxListKeys int
	benchmark   bool
	loglevel    string
	cpuprofile  string

	// globals
	log = logging.MustGetLogger("s3pd")
)

func init() {
	// if region is left as an empty string, AWS SDK will get the region from:
	// environment variables, AWS shared configuration file (~/.aws/config),
	// or AWS shared credentials file (~/.aws/credentials).
	flag.StringVar(&region, "region", "", "Force a specific S3 AWS Region endpoint to be used (Optional)")

	flag.UintVar(&downloaders, "downloaders", 10, "Number of concurrent s3 downloads (Default 10)")
	flag.UintVar(&threads, "threads", 5, "Number of threads used by each downloader (Default 5)")
	flag.Int64Var(&partsize, "partsize", 5*1024*1024, "bytes to assign each thread to download, (Deafult 5*1024*1024)")
	flag.IntVar(&maxListKeys, "maxlistkeys", 1000, "max number of object keys to return in each ls pagination request (Default 1000)")
	flag.BoolVar(&benchmark, "benchmark", false, "set to true to test raw download to ram speed (Default false)")
	flag.StringVar(&loglevel, "loglevel", "NOTICE", "Level of logging to expose, INFO, NOTICE, WARNING, ERROR. (Default \"NOTICE\")")
	flag.StringVar(&cpuprofile, "cpuprofile", "", "Writes cpu profile to specified filepath (Optional)")
}

func parseS3Path(path string) (bucket string, prefix string) {
	if !strings.HasPrefix(path, "s3://") {
		return "", ""
	}

	u, _ := url.Parse(source)
	bucket = u.Host
	prefix = u.Path

	// remove starting slash
	if strings.HasPrefix(prefix, "/") {
		prefix = prefix[1:]
	}
	return bucket, prefix
}

func parseFlags() error {
	flag.Usage = func() {
		w := os.Stdout
		fmt.Fprintf(w, "\033[1mDESCRIPTION:\033[0m\n")
		fmt.Fprintf(w, "3pd is a utility for downloading or uploading multiple S3 objects at a time using multiple threads\n\n")
		fmt.Fprintf(w, "\033[1mUSAGE:\033[0m\n")
		fmt.Fprintf(w, "s3pd [flags] [source] [destination]\n\n")
		fmt.Fprintf(w, "\033[1mEXAMPLES:\033[0m\n")
		fmt.Fprintf(w, "The following is how to download objects in mybucket with the prefix of mydataset/* to /mnt/scratch\n\n")
		fmt.Fprintf(w, "\ts3pd s3://mybucket/mydataset/* /mnt/scratch\n\n\n")
		fmt.Fprintf(w, "The following is how to download objects in mybucket with the prefix of mydataset/* to /mnt/scratch")
		fmt.Fprintf(w, "using the s3 api in us-east-2, and downloading 25 objects at a time\n\n")
		fmt.Fprintf(w, "\ts3pd s3://mybucket/mydataset/* /mnt/scratch --region=us-east-2 --downloaders=25\n\n\n")
		fmt.Fprintf(w, "The following is how to download objects in mybucket with the prefix of mydataset/* to /mnt/scratch")
		fmt.Fprintf(w, "using the s3 api in us-east-2, and downloading 25 objects at a time. With 5 threads used ")
		fmt.Fprintf(w, "to download each object. 125 concurrent s3 downloads total)\n\n")
		fmt.Fprintf(w, "\ts3pd s3://mybucket/mydataset/* /mnt/scratch --region=us-east-2 --downloaders=25 --threads=5\n\n")
		fmt.Fprintf(w, "\033[1mFLAGS:\033[0m\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	isHelp := len(args) == 1 && args[0] == "help"
	hasSourceAndDest := len(args) == 2 || (len(args) == 1 && benchmark)
	if isHelp && !hasSourceAndDest {
		return errors.New("Missing [source] and [destination]")
	}

	if !isHelp {
		source = args[0]
	}
	
	if !isHelp && !benchmark {
		destination = args[1]
	}

	// Error if both source & Destination are s3 paths
	if !isHelp && strings.HasPrefix(source, "s3://") && strings.HasPrefix(destination, "s3://") {
		err := "Cannot have s3 as both the source & destination.\n"
		err += "s3pd does not support moving objects between S3 buckets"
		return errors.New(err)
	}

	return nil
}

func main() {
	if err := parseFlags(); err != nil {
		fmt.Fprintln(os.Stderr, "\033[1;31m"+err.Error()+"\033[0m")
		os.Exit(1)
	}

	if flag.Args()[0] == "help" {
		flag.Usage()
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

	// Create new go-logging instance that sets logs to the declared level.
	lm := logging.AddModuleLevel(logging.NewLogBackend(os.Stderr, "", 0))
	logLevels := map[string]logging.Level{
		"DEBUG":    logging.DEBUG,
		"INFO":     logging.INFO,
		"NOTICE":   logging.NOTICE,
		"WARNING":  logging.WARNING,
		"ERROR":    logging.ERROR,
		"CRITICAL": logging.CRITICAL,
	}
	lm.SetLevel(logLevels[strings.ToUpper(loglevel)], "")
	logging.SetBackend(lm)

	if strings.HasPrefix(source, "s3://") {
		performDownload()
	} else if strings.HasPrefix(destination, "s3://") {
		panic("writes not yet implemented")
	}
}

func performDownload() {
	bucket, prefix := parseS3Path(source)

	download := Download{
		bucket:      bucket,
		prefix:      prefix,
		writepath:   destination,
		region:      region,
		downloaders: downloaders,
		threads:     threads,
		partsize:    partsize,
		maxListKeys: maxListKeys,
		bar:         pb.New(1), // putting bar size of 1 as a placeholder
	}

	if err := download.Start(context.Background()); err != nil {
		panic(err)
	}

	fmt.Printf("\nAverage throughput was: %0.4fGibps\n", download.Throughput())
}
