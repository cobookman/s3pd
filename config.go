package main

import (
	"os"
	flag "github.com/spf13/pflag"
	"fmt"
	"errors"
	"strings"
)

type Config struct  {
	// positional arguments
	source      string
	destination string

	// flags
	region      string
	workers     uint
	threads     uint
	partsize    int64
	maxList     int
	nics        string
	isBenchmark bool
	loglevel    string
	cpuprofile  string
}

func NewConfig(args []string) (c *Config, err error) {
	c = &Config{}
	err = c.parse(args)
	return c, err
}

func (c *Config) parse(args []string) (err error) {
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	
	// if region is left as an empty string, AWS SDK will get the region from:
	// environment variables, AWS shared configuration file (~/.aws/config),
	// or AWS shared credentials file (~/.aws/credentials).
	f.StringVar(&c.region, "region", "", "Force a specific S3 AWS Region endpoint to be used (Optional)")

	f.UintVar(&c.workers, "workers", 10, "Number of concurrent workers - concurrent API Calls (Default 10)")
	f.UintVar(&c.threads, "threads", 5, "Number of threads given to each worker (Default 5)")
	f.Int64Var(&c.partsize, "partsize", 5*1024*1024, "bytes to assign each thread to download, (Deafult 5*1024*1024)")
	f.IntVar(&c.maxList, "maxlist", 1000, "max number of objects/files to return in each list request (Default 1000)")
	f.BoolVar(&c.isBenchmark, "benchmark", false, "when set will download data temporarily to ram (Default false)")

	// Certain Ec2 instances such-as the p4d.24xl and dl1.24xl can provide in excess of 100Gibps of network throughput
	// by attaching 4 ENIs, each having its own distinct NetworkCardIndex.
	// When this local interfaces are provided, the program will round robin distribute HTTP requests across the multiple
	// interfaces, improving performance.
	f.StringVar(&c.nics, "nics", "", "to send load across multiple NICs, set to a list of network interfaces to LB across E.g. (--nics=en0,en1,en2,en3)")

	f.StringVar(&c.loglevel, "loglevel", "NOTICE", "Level of logging to expose, INFO, NOTICE, WARNING, ERROR. (Default \"NOTICE\")")
	f.StringVar(&c.cpuprofile, "cpuprofile", "", "Writes cpu profile to specified filepath")

	f.Usage = func() {
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
		fmt.Fprintf(w, "\ts3pd s3://mybucket/mydataset/* /mnt/scratch --region=us-east-2 --workers=25\n\n\n")
		fmt.Fprintf(w, "The following is how to download objects in mybucket with the prefix of mydataset/* to /mnt/scratch")
		fmt.Fprintf(w, "using the s3 api in us-east-2, and downloading 25 objects at a time. With 5 threads used ")
		fmt.Fprintf(w, "to download each object. 125 concurrent s3 downloads total)\n\n")
		fmt.Fprintf(w, "\ts3pd s3://mybucket/mydataset/* /mnt/scratch --region=us-east-2 --workers=25 --threads=5\n\n")
		fmt.Fprintf(w, "\033[1mFLAGS:\033[0m\n")
		flag.PrintDefaults()
	}

	if err := f.Parse(args[1:]); err != nil {
		return err
	}

	args = f.Args()
	isHelp := len(args) == 1 && args[0] == "help"
	if isHelp {
		f.Usage()
		os.Exit(0)
	}

	hasSourceAndDest := len(args) == 2 || (len(args) == 1 && c.isBenchmark)
	if !hasSourceAndDest {
		return errors.New("Missing [source] and [destination]")
	}
	c.source = args[0]

	if !c.isBenchmark {
		c.destination = args[1]
	}

	return nil
}

// Parses the input "en0,en1,en2,en3" nics
// and outputs them as an array of ["en0", "en1", "en2", "en3"]
func (c Config) NicsArr() (out []string) {
	if len(c.nics) == 0 {
		return out
	}

	s := c.nics
	if strings.HasSuffix(s, ",") {
		s = s[:len(s)-1]
	}

	return strings.Split(s, ",")
}
