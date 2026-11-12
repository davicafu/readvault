package content

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// WorkerConfig sets how the Worker downloads; zero values take the defaults.
type WorkerConfig struct {
	// Workers is how many downloads run at once.
	Workers int
	// Pause is the wait between two downloads of one worker, so sites and
	// the mirror are not flooded.
	Pause time.Duration
	// Idle is the wait before looking at an empty queue again.
	Idle time.Duration
	// Lease is how long a claimed download is kept before it is handed out
	// again; longer than a download can take.
	Lease time.Duration
}

// Worker downloads the text of queued articles in the background.
type Worker struct {
	svc    *Service
	queue  DownloadQueue
	cfg    WorkerConfig
	logger *slog.Logger
}

// NewWorker returns a Worker that downloads with svc.
func NewWorker(svc *Service, queue DownloadQueue, cfg WorkerConfig) *Worker {
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	if cfg.Pause <= 0 {
		cfg.Pause = time.Second
	}
	if cfg.Idle <= 0 {
		cfg.Idle = 5 * time.Second
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 2 * time.Minute
	}
	return &Worker{svc: svc, queue: queue, cfg: cfg, logger: slog.Default()}
}

// Run downloads until ctx is cancelled and returns once every download has
// stopped. A download cut short is handed out again when its claim runs out.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range w.cfg.Workers {
		wg.Go(func() { w.loop(ctx) })
	}
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context) {
	for {
		wait := w.cfg.Idle
		job, ok, err := w.queue.Claim(ctx, w.cfg.Lease)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			w.logger.Error("claiming a text download", slog.Any("error", err))
		case ok:
			w.download(ctx, job)
			wait = w.cfg.Pause
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (w *Worker) download(ctx context.Context, job DownloadJob) {
	log := w.logger.With(slog.String("article_id", job.ArticleID))
	err := w.svc.downloadMissing(ctx, job.UserID, job.ArticleID)
	var fe *FetchError
	switch {
	case ctx.Err() != nil:
		// Stopping: the claim runs out and the download is tried again.
		return
	case err == nil:
		log.Info("text downloaded")
	case errors.As(err, &fe):
		// Recorded on the article; the person can download it again.
		log.Info("text download failed", slog.String("problem", fe.Problem))
	case errors.Is(err, ErrNotFound):
		// The article was deleted.
	default:
		// Left queued: tried again when the claim runs out.
		log.Error("downloading text", slog.Any("error", err))
		return
	}
	if err := w.queue.Done(ctx, job); err != nil {
		log.Error("removing a finished text download", slog.Any("error", err))
	}
}
