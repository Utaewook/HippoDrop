package worker

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"tardis/internal/queue"
	"tardis/internal/storage"
)

type Pool struct {
	db          *queue.DB
	provider    storage.Provider
	taskChan    chan *queue.Task
	poolSize    int
	maxRetries  int
	rateLimiter *rate.Limiter
	wg          sync.WaitGroup
}

func NewPool(db *queue.DB, provider storage.Provider, taskChan chan *queue.Task, poolSize, limitPerSec, maxRetries int) *Pool {
	return &Pool{
		db:          db,
		provider:    provider,
		taskChan:    taskChan,
		poolSize:    poolSize,
		maxRetries:  maxRetries,
		rateLimiter: rate.NewLimiter(rate.Limit(limitPerSec), limitPerSec), // Token bucket
	}
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.poolSize; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
}

func (p *Pool) Wait() {
	p.wg.Wait()
}

func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()
	log.Printf("Worker %d started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d shutting down...", id)
			return
		case t := <-p.taskChan:
			p.processTask(ctx, id, t)
		}
	}
}

func (p *Pool) processTask(ctx context.Context, workerID int, t *queue.Task) {
	// Block until a rate limit token is available
	if err := p.rateLimiter.Wait(ctx); err != nil {
		log.Printf("Worker %d rate limiter canceled: %v", workerID, err)
		return
	}

	var err error
	if t.Type == "upload" {
		err = p.provider.Upload(ctx, t.LocalPath, t.RemotePath)
	} else if t.Type == "download" {
		err = p.provider.Download(ctx, t.RemotePath, t.LocalPath)
	} else {
		log.Printf("Worker %d unknown task type: %s", workerID, t.Type)
		p.failTask(ctx, t.ID, "unknown task type")
		return
	}

	if err != nil {
		log.Printf("Worker %d failed task %s: %v", workerID, t.ID, err)
		p.handleFailure(ctx, t, err)
		return
	}

	// Success
	p.completeTask(ctx, t.ID)
}

func (p *Pool) handleFailure(ctx context.Context, t *queue.Task, err error) {
	if t.RetryCount >= p.maxRetries {
		p.failTask(ctx, t.ID, err.Error())
		return
	}

	// Exponential Backoff
	backoffSec := math.Pow(2, float64(t.RetryCount))
	log.Printf("Task %s backing off for %v seconds", t.ID, backoffSec)
	
	select {
	case <-time.After(time.Duration(backoffSec) * time.Second):
	case <-ctx.Done():
		// If context canceled during backoff, revert to pending so it can resume later
		p.revertToPending(context.Background(), t.ID)
		return
	}

	// Increment retry count and revert to pending
	_, dbErr := p.db.ExecContext(ctx, "UPDATE tasks SET status = 'pending', retry_count = retry_count + 1, updated_at = CURRENT_TIMESTAMP WHERE task_id = ?", t.ID)
	if dbErr != nil {
		log.Printf("Failed to increment retry for task %s: %v", t.ID, dbErr)
	}
}

func (p *Pool) completeTask(ctx context.Context, taskID string) {
	_, err := p.db.ExecContext(ctx, "UPDATE tasks SET status = 'done', updated_at = CURRENT_TIMESTAMP WHERE task_id = ?", taskID)
	if err != nil {
		log.Printf("Failed to mark task %s as done: %v", taskID, err)
	}
}

func (p *Pool) failTask(ctx context.Context, taskID string, errMsg string) {
	_, err := p.db.ExecContext(ctx, "UPDATE tasks SET status = 'failed', error_msg = ?, updated_at = CURRENT_TIMESTAMP WHERE task_id = ?", errMsg, taskID)
	if err != nil {
		log.Printf("Failed to mark task %s as failed: %v", taskID, err)
	}
}

func (p *Pool) revertToPending(ctx context.Context, taskID string) {
	_, _ = p.db.ExecContext(ctx, "UPDATE tasks SET status = 'pending', updated_at = CURRENT_TIMESTAMP WHERE task_id = ?", taskID)
}
