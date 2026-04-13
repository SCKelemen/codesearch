package advisory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultPollInterval = 5 * time.Minute
	defaultBatchSize    = 100
)

// Store persists normalized advisories. This mirrors the vuln.AdvisoryStore
// interface but is defined here to avoid circular imports.
type Store interface {
	Put(ctx context.Context, adv Advisory) error
	PutBatch(ctx context.Context, advs []Advisory) error
}

// PipelineOptions configures the advisory ingestion pipeline.
type PipelineOptions struct {
	PollInterval time.Duration                // how often to poll feeds (default: 5 minutes)
	BatchSize    int                          // max advisories per batch write (default: 100)
	Logger       *slog.Logger                 // structured logger (default: slog.Default())
	OnError      func(feed string, err error) // error callback
	OnIngested   func(feed string, count int) // success callback
}

// Pipeline continuously polls threat intelligence feeds and stores normalized
// advisories.
type Pipeline struct {
	feeds []Feed
	store Store
	opts  PipelineOptions

	mu       sync.Mutex
	lastPoll map[string]time.Time
	running  bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewPipeline returns a new advisory ingestion pipeline with defaults applied.
func NewPipeline(store Store, feeds []Feed, opts PipelineOptions) *Pipeline {
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultPollInterval
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &Pipeline{
		feeds:    append([]Feed(nil), feeds...),
		store:    store,
		opts:     opts,
		lastPoll: make(map[string]time.Time, len(feeds)),
	}
}

// Start begins the polling loop in a background goroutine.
func (p *Pipeline) Start(ctx context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.running = true
	p.wg.Add(1)
	p.mu.Unlock()

	go func() {
		defer p.wg.Done()
		defer func() {
			p.mu.Lock()
			p.running = false
			p.cancel = nil
			p.mu.Unlock()
		}()

		p.opts.Logger.Info("starting advisory pipeline", "feed_count", len(p.feeds), "poll_interval", p.opts.PollInterval)
		if err := p.pollAll(loopCtx); err != nil {
			p.opts.Logger.Error("advisory pipeline poll failed", "error", err)
		}

		ticker := time.NewTicker(p.opts.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-loopCtx.Done():
				p.opts.Logger.Info("stopping advisory pipeline")
				return
			case <-ticker.C:
				if err := p.pollAll(loopCtx); err != nil {
					p.opts.Logger.Error("advisory pipeline poll failed", "error", err)
				}
			}
		}
	}()
}

// Stop gracefully shuts down the pipeline.
func (p *Pipeline) Stop() {
	p.mu.Lock()
	cancel := p.cancel
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	p.wg.Wait()
}

// PollOnce runs a single poll cycle synchronously.
func (p *Pipeline) PollOnce(ctx context.Context) error {
	return p.pollAll(ctx)
}

func (p *Pipeline) pollAll(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(p.feeds))

	for _, feed := range p.feeds {
		feed := feed
		wg.Add(1)
		go func() {
			defer wg.Done()

			since := p.LastPollTime(feed.Name())
			pollStartedAt := time.Now().UTC()
			advisories, err := feed.Fetch(ctx, since)
			if err != nil {
				wrapped := fmt.Errorf("fetch %s: %w", feed.Name(), err)
				p.opts.Logger.ErrorContext(ctx, "failed to fetch advisories", "feed", feed.Name(), "error", err)
				if p.opts.OnError != nil {
					p.opts.OnError(feed.Name(), err)
				}
				errs <- wrapped
				return
			}

			if err := p.putAdvisories(ctx, advisories); err != nil {
				wrapped := fmt.Errorf("store %s: %w", feed.Name(), err)
				p.opts.Logger.ErrorContext(ctx, "failed to store advisories", "feed", feed.Name(), "error", err)
				if p.opts.OnError != nil {
					p.opts.OnError(feed.Name(), err)
				}
				errs <- wrapped
				return
			}

			p.setLastPollTime(feed.Name(), pollStartedAt)
			p.opts.Logger.InfoContext(ctx, "ingested advisories", "feed", feed.Name(), "count", len(advisories))
			if p.opts.OnIngested != nil {
				p.opts.OnIngested(feed.Name(), len(advisories))
			}
		}()
	}

	wg.Wait()
	close(errs)

	var joined []error
	for err := range errs {
		joined = append(joined, err)
	}
	return errors.Join(joined...)
}

func (p *Pipeline) putAdvisories(ctx context.Context, advisories []Advisory) error {
	if len(advisories) == 0 {
		return nil
	}
	for start := 0; start < len(advisories); start += p.opts.BatchSize {
		end := start + p.opts.BatchSize
		if end > len(advisories) {
			end = len(advisories)
		}
		if err := p.store.PutBatch(ctx, advisories[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pipeline) setLastPollTime(feedName string, ts time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastPoll[feedName] = ts
}

// LastPollTime returns when a feed was last successfully polled.
func (p *Pipeline) LastPollTime(feedName string) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastPoll[feedName]
}

// Running reports whether the pipeline is currently active.
func (p *Pipeline) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}
