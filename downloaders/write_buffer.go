package downloaders

import (
	"github.com/cheggaaa/pb/v3"
	"io"
	"io/ioutil"
)

type DiscardWriteBuffer struct {
}

func NewDiscardWriteBuffer() *DiscardWriteBuffer {
	return &DiscardWriteBuffer{}
}

func (w DiscardWriteBuffer) WriteAt(p []byte, offset int64) (n int, err error) {
	return ioutil.Discard.Write(p)
}

type LogProgressWriteBuffer struct {
	bar *pb.ProgressBar
	w   io.WriterAt
}

func NewLogProgressWriteBuffer(bar *pb.ProgressBar, w io.WriterAt) *LogProgressWriteBuffer {
	return &LogProgressWriteBuffer{bar: bar, w: w}
}

func (l LogProgressWriteBuffer) WriteAt(p []byte, offset int64) (n int, err error) {
	l.bar.Add64(int64(len(p)))
	return l.w.WriteAt(p, offset)
}
