package advisory

import (
	"context"
	"time"
)

// Feed is a source of vulnerability advisories.
type Feed interface {
	// Name returns the feed identifier.
	Name() string
	// Fetch retrieves advisories modified since the given time.
	Fetch(ctx context.Context, since time.Time) ([]Advisory, error)
}
