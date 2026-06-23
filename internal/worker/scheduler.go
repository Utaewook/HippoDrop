package worker

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"tardis/internal/queue"
)

// Scheduler polls the database for pending tasks and pushes them to a channel.
type Scheduler struct {
	db       *queue.DB
	taskChan chan *queue.Task
	paused   atomic.Bool
}

func NewScheduler(db *queue.DB, taskChan chan *queue.Task) *Scheduler {
	return &Scheduler{
		db:       db,
		taskChan: taskChan,
	}
}

func (s *Scheduler) Pause() {
	s.paused.Store(true)
	log.Println("Scheduler paused")
}

func (s *Scheduler) Resume() {
	s.paused.Store(false)
	log.Println("Scheduler resumed")
}

func (s *Scheduler) IsPaused() bool {
	return s.paused.Load()
}

// Start begins the polling loop. It runs until ctx is canceled.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Scheduler shutting down...")
			return
		case <-ticker.C:
			s.dispatchPendingTasks(ctx)
		}
	}
}

func (s *Scheduler) dispatchPendingTasks(ctx context.Context) {
	if s.IsPaused() {
		return
	}
	// Query pending tasks (limit to channel capacity to avoid blocking too long)
	limit := cap(s.taskChan)
	rows, err := s.db.QueryContext(ctx, "SELECT task_id, type, local_path, remote_path, status, retry_count, COALESCE(callback_url, ''), callback_status, callback_retry_count FROM tasks WHERE status = 'pending' ORDER BY created_at ASC LIMIT ?", limit)
	if err != nil {
		log.Printf("Scheduler failed to query pending tasks: %v", err)
		return
	}
	defer rows.Close()

	var tasks []*queue.Task
	for rows.Next() {
		t := &queue.Task{}
		if err := rows.Scan(&t.ID, &t.Type, &t.LocalPath, &t.RemotePath, &t.Status, &t.RetryCount, &t.CallbackURL, &t.CallbackStatus, &t.CallbackRetryCount); err != nil {
			log.Printf("Scheduler failed to scan task: %v", err)
			continue
		}
		tasks = append(tasks, t)
	}

	for _, t := range tasks {
		// Update status to 'running' BEFORE pushing to channel to prevent duplicate dispatch
		_, err := s.db.ExecContext(ctx, "UPDATE tasks SET status = 'running', updated_at = CURRENT_TIMESTAMP WHERE task_id = ? AND status = 'pending'", t.ID)
		if err != nil {
			log.Printf("Scheduler failed to mark task %s as running: %v", t.ID, err)
			continue
		}

		select {
		case s.taskChan <- t:
			// successfully dispatched
		case <-ctx.Done():
			// shutdown during dispatch
			return
		}
	}
}
