package downloaders

import (
	"context"
	"errors"
	"github.com/cheggaaa/pb/v3"
	"github.com/op/go-logging"
	"golang.org/x/sync/errgroup"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"
)

type FilesystemDownload struct {
	Readpath    string
	Writepath   string
	Workers     uint
	Threads     uint
	Partsize    int64
	MaxList     int
	IsBenchmark bool
	Bar         *pb.ProgressBar
	Log         *logging.Logger
	StartTime   time.Time
}

type FileCopyJob struct {
	Readpath string
	Filepath string
	Name     string
	Size     int64
}

func (d *FilesystemDownload) Start(ctx context.Context) error {
	d.StartTime = time.Now()

	// Instantiate download workers
	// Set job's channel length to 3x max files we'll get in a list op
	// if the job queue ends up filling up, we'll stall doing additional list ops until the queue has more messages completed
	jobs := make(chan FileCopyJob, d.MaxList*3)
	eg, ctx := errgroup.WithContext(ctx)
	for w := 1; w <= int(d.Workers); w++ {
		eg.Go(func() error {
			return d.worker(int(w), jobs)
		})
	}

	// Start the progress bar
	d.Bar.Start()

	// Queue up download tasks
	if err := d.list(jobs); err != nil {
		// if error clean up workers and return the error
		close(jobs)
		ctx.Done()
		return err
	}

	// Indicate that we listed every single file and there's no more files needing to be queued
	close(jobs)

	// Wait till all downloads finish, or until we get our first error
	if err := eg.Wait(); err != nil {
		return err
	}

	d.Bar.Finish()
	return nil
}

func (d FilesystemDownload) list(jobs chan<- FileCopyJob) error {
	d.Log.Debugf("Listing files under: %s", d.Readpath)

	// WalkDir is fast enough for our needs
	// WalkDir uses a single thread, and is not multi-threaded
	// https://engineering.kablamo.com.au/posts/2021/quick-comparison-between-go-file-walk-implementations
	var numBytes int64 = 0
	return filepath.WalkDir(d.Readpath, func(path string, f fs.DirEntry, err error) error {
		// propegate error, and stop traversing filesystem
		if err != nil {
			return err
		}

		if !f.IsDir() {
			fileinfo, err := f.Info()
			if err != nil {
				return err
			}

			jobs <- FileCopyJob{
				Readpath: d.Readpath,
				Filepath: path,
				Name:     f.Name(),
				Size:     fileinfo.Size(),
			}
			numBytes += fileinfo.Size()
			d.Bar.SetTotal(numBytes)
		}
		return nil
	})
}

type PartCopyJob struct {
	Source      *os.File
	Destination *os.File
	Offset      int64
}

func (d FilesystemDownload) worker(id int, jobs <-chan FileCopyJob) error {
	// startup concurrent writers
	// jobs := make(chan FileCopyJob, d.Threads)
	// for t := 1; w < int(d.Threads); t++ {

	// }

	for j := range jobs {
		relativePath := j.Filepath[len(j.Readpath):]
		absoluteWritepath := path.Join(d.Writepath, relativePath)
		absoluteReadpath := j.Filepath
		d.Log.Debugf("Job in worker %d reading file %s and writing it to %s of size %dBytes",
			id, absoluteReadpath, absoluteWritepath, j.Size)

		// ensure any missing dirs are created. MkdirAll returns nil if folder already exists
		if err := os.MkdirAll(filepath.Dir(absoluteWritepath), os.ModePerm); err != nil {
			return err
		}

		source, err := os.Open(absoluteReadpath)
		if err != nil {
			return err
		}

		var destination *os.File = nil
		if !d.IsBenchmark {
			var err error
			destination, err = os.OpenFile(absoluteWritepath, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
		}

		// Startup the copy threadpool
		eg, _ := errgroup.WithContext(context.Background())
		partsToCopy := make(chan PartCopyJob, d.Threads*2)
		for t := 1; t <= int(d.Threads); t++ {
			eg.Go(func() error {
				return d.partCopyWorker(int(t), partsToCopy)
			})
		}

		// Schedule parts to be copied by the threadpool
		var offset int64 = 0
		for offset < j.Size {
			partsToCopy <- PartCopyJob{
				Source:      source,
				Destination: destination,
				Offset:      offset,
			}
			offset += d.Partsize
		}
		close(partsToCopy)

		// Block & wait till all parts are copied
		// If err occurs pass it back
		if err := eg.Wait(); err != nil {
			return err
		}
	}
	return nil
}

// Copies the parts sent to the partsToCopy channel
func (d FilesystemDownload) partCopyWorker(id int, partsToCopy <-chan PartCopyJob) error {
	buffer := make([]byte, d.Partsize, d.Partsize)
	for p := range partsToCopy {
		d.Log.Debugf("Received job to copy from %v to %v at offset: %d",
			p.Source, p.Destination, p.Offset)

		bytesRead, err := p.Source.ReadAt(buffer, p.Offset)
		if err != nil && err != io.EOF {
			return err
		}

		if d.IsBenchmark {
			ioutil.Discard.Write(buffer[:bytesRead])
		} else {
			bytesWritten, err := p.Destination.WriteAt(buffer[:bytesRead], p.Offset)
			if err != nil {
				return err
			}

			if bytesRead != bytesWritten {
				return errors.New("Different number of bytes read & write")
			}
		}
		// Log downloaded data
		d.Bar.Add(bytesRead)
	}
	return nil
}

func (d FilesystemDownload) Throughput() float64 {
	return float64(d.Bar.Total()) * 8 / 1024 / 1024 / 1024 / time.Since(d.StartTime).Seconds()
}
