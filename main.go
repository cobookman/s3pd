package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/cheggaaa/pb/v3"
	"github.com/cobookman/s3-parallel-downloader/downloaders"
	"github.com/op/go-logging"
	"net/url"
	"os"
	"runtime/pprof"
	"strings"
)

func main() {
	c, err := NewConfig(os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "\033[1;31m"+err.Error()+"\033[0m")
		os.Exit(1)
	}

	// Let user know if benchmark mode is enabled
	// NOTE: When in benchmark mode this avoids needing to do filesystem IO in creating missing directories
	// and opening file for writing.
	// This is a savings of roughly 5 to 10ms per Object download request
	if c.isBenchmark {
		fmt.Println("Benchmark mode, data being written to temporary in memory object")
	}

	// If cpu profiling flag set, enable pprof cpu profiling - this will have a minor performance hit
	// every few hundred cpu cycles a snapshot of the program state will be taken
	if len(c.cpuprofile) != 0 {
		f, err := os.Create(c.cpuprofile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "\033[1;31m"+err.Error()+"\033[0m")
			os.Exit(1)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Create new go-logging instance that sets logs to the declared level.
	log := logging.MustGetLogger("s3pd")
	lm := logging.AddModuleLevel(logging.NewLogBackend(os.Stderr, "", 0))
	logLevels := map[string]logging.Level{
		"DEBUG":    logging.DEBUG,
		"INFO":     logging.INFO,
		"NOTICE":   logging.NOTICE,
		"WARNING":  logging.WARNING,
		"ERROR":    logging.ERROR,
		"CRITICAL": logging.CRITICAL,
	}
	lm.SetLevel(logLevels[strings.ToUpper(c.loglevel)], "")
	logging.SetBackend(lm)

	bar := pb.New(1) // putting bar size of 1 as a placeholder
	bar.SetWriter(os.Stdout)
	bar.Set(pb.SIBytesPrefix, false)
	bar.Set(pb.Bytes, true)

	d, err := getDownloader(c, log, bar)
	if err != nil {
		panic(err)
	}

	// Blocks until download is completed or on first error received
	if err := d.Start(context.Background()); err != nil {
		panic(err)
	}

	fmt.Printf("\nAverage throughput was: %0.4fGibps\n", d.Throughput())
}

// Parses the S3 bucket and object prefix from a string in format of "s3://bucket/prefix"
func parseS3Path(path string) (bucket string, prefix string) {
	if !strings.HasPrefix(path, "s3://") {
		return "", ""
	}

	u, _ := url.Parse(path)
	bucket = u.Host
	prefix = u.Path

	// remove starting slash
	if strings.HasPrefix(prefix, "/") {
		prefix = prefix[1:]
	}
	return bucket, prefix
}

// Returns a downloader for the given source
func getDownloader(c *Config, log *logging.Logger, bar *pb.ProgressBar) (downloaders.Downloader, error) {
	isSourceS3 := strings.HasPrefix(c.source, "s3://")
	isDestinationS3 := strings.HasPrefix(c.destination, "s3://")

	if isSourceS3 && !isDestinationS3 {
		bucket, prefix := parseS3Path(c.source)
		d := downloaders.S3Download{
			Bucket:      bucket,
			Prefix:      prefix,
			Writepath:   c.destination,
			Region:      c.region,
			Workers:     c.workers,
			Threads:     c.threads,
			Partsize:    c.partsize,
			MaxList:     c.maxList,
			IsBenchmark: c.isBenchmark,
			NICs:        c.NicsArr(),
			Log:         log,
			Bar:         bar,
		}
		return &d, nil
	}

	if isSourceS3 && isDestinationS3 {
		return nil, errors.New("moves between S3 buckets not implemented")
	}

	if !isSourceS3 && isDestinationS3 {
		return nil, errors.New("Uploads to S3 buckets not implemented")
	}

	if !isSourceS3 && !isDestinationS3 {
		d := downloaders.FilesystemDownload{
			Readpath:    c.source,
			Writepath:   c.destination,
			Workers:     c.workers,
			Threads:     c.threads,
			Partsize:    c.partsize,
			MaxList:     c.maxList,
			IsBenchmark: c.isBenchmark,
			Log:         log,
			Bar:         bar,
		}
		return &d, nil
	}

	return nil, errors.New("Unsupported cp operation")
}
