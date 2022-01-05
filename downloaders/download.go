package downloaders

import (
	"context"
)

type Downloader interface {
	Throughput() float64
	Start(ctx context.Context) error
}
