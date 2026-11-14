package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/content"
)

// maxDownloadAttempts bounds how often a download whose claim ran out is
// handed out again.
const maxDownloadAttempts = 3

// DownloadQueue implements content.DownloadQueue on the content_downloads
// table. Claims skip rows other workers hold, so several workers and API
// processes can share it.
type DownloadQueue struct {
	pool  *pgxpool.Pool
	delay time.Duration
}

// NewDownloadQueue returns a DownloadQueue backed by pool. An article is
// handed out only once it has been queued for delay, which leaves a link
// the web app downloads right after adding it to that download: by then
// the article has its text or a recorded failure and the worker skips it.
func NewDownloadQueue(pool *pgxpool.Pool, delay time.Duration) *DownloadQueue {
	return &DownloadQueue{pool: pool, delay: delay}
}

// Claim implements content.DownloadQueue.
func (q *DownloadQueue) Claim(ctx context.Context, lease time.Duration) (content.DownloadJob, bool, error) {
	// Downloads that ran out of attempts are dropped; the article keeps no
	// text and the person can download it from its page.
	if _, err := q.pool.Exec(ctx, `DELETE FROM content_downloads
		WHERE attempts >= $1 AND claimed_until < now()`, maxDownloadAttempts); err != nil {
		return content.DownloadJob{}, false, fmt.Errorf("dropping exhausted downloads: %w", err)
	}

	var job content.DownloadJob
	err := q.pool.QueryRow(ctx, `WITH next AS (
			SELECT article_id FROM content_downloads
			WHERE attempts < $1 AND (claimed_until IS NULL OR claimed_until < now())
			  AND queued_at <= now() - $3 * interval '1 millisecond'
			ORDER BY queued_at, article_id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE content_downloads d
		SET attempts = d.attempts + 1, claimed_until = now() + $2 * interval '1 millisecond'
		FROM next WHERE d.article_id = next.article_id
		RETURNING d.article_id, d.user_id`,
		maxDownloadAttempts, lease.Milliseconds(), q.delay.Milliseconds()).Scan(&job.ArticleID, &job.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return content.DownloadJob{}, false, nil
	}
	if err != nil {
		return content.DownloadJob{}, false, fmt.Errorf("claiming a download: %w", err)
	}
	return job, true, nil
}

// Done implements content.DownloadQueue.
func (q *DownloadQueue) Done(ctx context.Context, job content.DownloadJob) error {
	if _, err := q.pool.Exec(ctx, `DELETE FROM content_downloads WHERE article_id = $1 AND user_id = $2`,
		job.ArticleID, job.UserID); err != nil {
		return fmt.Errorf("removing a download: %w", err)
	}
	return nil
}
