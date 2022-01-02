package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/cheggaaa/pb/v3"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Download struct {
	bucket      string
	prefix      string
	writepath   string
	region      string
	downloaders uint
	threads     uint
	partsize    int64
	maxListKeys int
	bar         *pb.ProgressBar
	start       time.Time
}

func (d *Download) Start(ctx context.Context) error {
	d.start = time.Now()

	// Create s3 client
	// Note, if region is an empty string, then will ignore the region value and use the region from system config
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(d.region))
	if err != nil {
		return err
	}
	client := s3.NewFromConfig(cfg)

	// Instantiate download workers
	// Set job's channel length to 3x max objects we'll get in a list op
	// if the job queue ends up filling up, we'll stall doing additional list ops until the queue has more messages completed
	jobs := make(chan s3types.Object, maxListKeys*3)
	eg, ctx := errgroup.WithContext(ctx)
	downloader := s3manager.NewDownloader(client, func(s3md *s3manager.Downloader) {
		s3md.PartSize = d.partsize
		s3md.Concurrency = int(d.threads)
		s3md.BufferProvider = s3manager.NewPooledBufferedWriterReadFromProvider(int(d.partsize))
	})
	for w := 1; w < int(d.downloaders); w++ {
		eg.Go(func() error {
			return d.worker(int(w), downloader, jobs)
		})
	}

	// Display the progress bar
	d.bar.SetWriter(os.Stdout)
	d.bar.Set(pb.SIBytesPrefix, false)
	d.bar.Set(pb.Bytes, true)
	d.bar.Start()

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

	// Done downloading, print final throughput estimate & show progress bar as finished
	d.bar.Finish()
	return nil
}

func (d Download) list(client *s3.Client, jobs chan<- s3types.Object) error {
	log.Debugf("Listing objects with the prefix of s3://%s/%s\n", d.bucket, d.prefix)
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket:  &d.bucket,
		Prefix:  &d.prefix,
		MaxKeys: int32(maxListKeys),
	})

	var numBytes int64 = 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return err
		}

		log.Debugf("Scheduling %d objects to be downloaded\n", len(page.Contents))
		for _, item := range page.Contents {
			jobs <- item
			numBytes += item.Size // size in Bytes
		}
		d.bar.SetTotal(numBytes)
	}
	return nil
}

func (d Download) worker(id int, downloader *s3manager.Downloader, jobs <-chan s3types.Object) error {
	for j := range jobs {
		objWritePath := fmt.Sprintf("%s/%s", d.writepath, *j.Key)

		log.Debugf("worker-%d writing s3://%s/%s to %s [%.2fMiB]\n",
			id,
			d.bucket, *j.Key,
			objWritePath,
			(float64(j.Size) / 1024 / 1024))

		var w io.WriterAt
		if benchmark {
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

		w = NewLogProgressWriteBuffer(d.bar, w)

		_, err := downloader.Download(context.Background(), w, &s3.GetObjectInput{
			Bucket: aws.String(d.bucket),
			Key:    j.Key,
		})

		if err != nil {
			return err
		}
	}
	return nil
}

func (d Download) Throughput() float64 {
	return float64(d.bar.Total()) * 8 / 1024 / 1024 / 1024 / time.Since(d.start).Seconds()
}
