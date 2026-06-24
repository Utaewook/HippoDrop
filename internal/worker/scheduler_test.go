package worker

import (
	"context"
	"testing"
	"time"

	"hippodrop/internal/queue"
)

func setupTestDB(t *testing.T) (*queue.DB, func()) {
	tempDir := t.TempDir()
	db, err := queue.Open(tempDir)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return db, func() { db.Close() }
}

func TestScheduler_DispatchPendingTasks(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	// 1. Insert a pending task and a done task
	_, err := db.Exec(`
		INSERT INTO tasks (task_id, type, local_path, remote_path, status)
		VALUES 
			('task-pending-1', 'upload', '/local/1', '/remote/1', 'pending'),
			('task-done-2', 'download', '/local/2', '/remote/2', 'done')
	`)
	if err != nil {
		t.Fatalf("failed to insert test tasks: %v", err)
	}

	taskChan := make(chan *queue.Task, 5)
	scheduler := NewScheduler(db, taskChan)

	// 2. Dispatch pending tasks
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	scheduler.dispatchPendingTasks(ctx)

	// 3. Verify channel has 1 item (the pending one)
	if len(taskChan) != 1 {
		t.Fatalf("expected 1 task in channel, got %d", len(taskChan))
	}

	dispatchedTask := <-taskChan
	if dispatchedTask.ID != "task-pending-1" {
		t.Errorf("expected dispatched task ID to be 'task-pending-1', got '%s'", dispatchedTask.ID)
	}

	// 4. Verify task state in database is updated to 'running'
	var status string
	err = db.QueryRow("SELECT status FROM tasks WHERE task_id = 'task-pending-1'").Scan(&status)
	if err != nil {
		t.Fatalf("failed to query status of task-pending-1: %v", err)
	}
	if status != "running" {
		t.Errorf("expected status to be 'running', got '%s'", status)
	}
}

func TestScheduler_StartShutdown(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	taskChan := make(chan *queue.Task, 5)
	scheduler := NewScheduler(db, taskChan)

	ctx, cancel := context.WithCancel(context.Background())
	
	// Start scheduler in a separate goroutine
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Wait briefly and cancel context to shutdown scheduler
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Clean shutdown
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler failed to shut down on context cancel")
	}
}
