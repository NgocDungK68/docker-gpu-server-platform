package ports

import (
	"context"
	"io"
	"time"
)

// ObjectStore keeps binary outputs outside the operational repository.
type ObjectStore interface {
	Put(ctx context.Context, key string, body io.Reader, size int64) (uri string, err error)
	DownloadURL(ctx context.Context, uri string, expires time.Duration) (string, error)
}
