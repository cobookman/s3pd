package downloaders

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/cheggaaa/pb/v3"
	"golang.org/x/sync/errgroup"
	"io"
	"github.com/op/go-logging"
	"os"
	"path/filepath"
	"time"
)

type S3Download struct {
	Bucket      string
	Prefix      string
	Writepath   string
	Region      string
	Workers uint
	Threads     uint
	Partsize    int64
	MaxList int
	IsBenchmark	bool
	Bar         *pb.ProgressBar
	Log			*logging.Logger
	StartTime       time.Time
}

func (d *S3Download) Start(ctx context.Context) error {
	d.StartTime = time.Now()

	// Create s3 client
	// Note, if region is an empty string, then will ignore the region value and use the region from system config
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(d.Region))
	if err != nil {
		return err
	}
	client := s3.NewFromConfig(cfg)

	// Instantiate download workers
	// Set job's channel length to 3x max objects we'll get in a list op
	// if the job queue ends up filling up, we'll stall doing additional list ops until the queue has more messages completed
	jobs := make(chan s3types.Object, d.MaxList*3)
	eg, ctx := errgroup.WithContext(ctx)
	downloader := s3manager.NewDownloader(client, func(s3md *s3manager.Downloader) {
		s3md.PartSize = d.Partsize
		s3md.Concurrency = int(d.Threads)
		s3md.BufferProvider = s3manager.NewPooledBufferedWriterReadFromProvider(int(d.Partsize))
	})
	for w := 1; w <= int(d.Workers); w++ {
		eg.Go(func() error {
			return d.worker(int(w), downloader, jobs)
		})
	}
	
	// Start the progress bar
	d.Bar.Start()
	
	// Queue up download tasks
	if err := d.list(client, jobs); err != nil {
		// if error clean up workers and return the error
		close(jobs)
		ctx.Done()
		return err
	}

	// Indicate that we listed every single object and there's no more objs needing to be queued
	close(jobs)

	// Wait till all downloads finish, or until we get our first error
	if err := eg.Wait(); err != nil {
		return err
	}

	d.Bar.Finish()
	return nil
}

func (d S3Download) list(client *s3.Client, jobs chan<- s3types.Object) error {
	d.Log.Debugf("Listing objects with the prefix of s3://%s/%s\n", d.Bucket, d.Prefix)
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket:  &d.Bucket,
		Prefix:  &d.Prefix,
		MaxKeys: int32(d.MaxList),
	})

	var numBytes int64 = 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return err
		}

		d.Log.Debugf("Scheduling %d objects to be downloaded\n", len(page.Contents))
		for _, item := range page.Contents {
			jobs <- item
			numBytes += item.Size // size in Bytes
		}
		d.Bar.SetTotal(numBytes)
	}
	return nil
}

func (d S3Download) worker(id int, downloader *s3manager.Downloader, jobs <-chan s3types.Object) error {
	// filepath.Dir returns "." if there's no dir in the path
 	prefixDir := filepath.Dir(d.Prefix)
	if prefixDir == "." {
		prefixDir = ""
	}
	
	for j := range jobs {
		objWritePath := filepath.Join(d.Writepath, (*j.Key)[len(prefixDir):])
		
		d.Log.Debugf("worker-%d writing s3://%s/%s to %s [%.2fMiB]\n",
			id,
			d.Bucket, *j.Key,
			objWritePath,
			(float64(j.Size) / 1024 / 1024))

		var w io.WriterAt
		if d.IsBenchmark {
			w = NewDiscardWriteBuffer()
		} else {
			// ensure dir is created. MkdirAll returns nil if folder already exists
			if err := os.MkdirAll(filepath.Dir(objWritePath), os.ModePerm); err != nil {
				return err
			}

			var err error
			w, err = os.Create(objWritePath)
			if err != nil {
				return err
			}
		}

		w = NewLogProgressWriteBuffer(d.Bar, w)

		_, err := downloader.Download(context.Background(), w, &s3.GetObjectInput{
			Bucket: aws.String(d.Bucket),
			Key:    j.Key,
		})

		if err != nil {
			return err
		}
	}
	return nil
}

func (d S3Download) Throughput() float64 {
	return float64(d.Bar.Total()) * 8 / 1024 / 1024 / 1024 / time.Since(d.StartTime).Seconds()
}