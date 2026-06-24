package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hippodrop/internal/queue"
)

// MockProvider is a mock implementation of storage.Provider
type MockProvider struct {
	UploadFunc    func(ctx context.Context, localPath, remotePath string) error
	DownloadFunc  func(ctx context.Context, remotePath, localPath string) error
	GetPathIDFunc func(ctx context.Context, path string, createIfMissing bool) (string, error)
}

func (m *MockProvider) Upload(ctx context.Context, localPath, remotePath string) error {
	if m.UploadFunc != nil {
		return m.UploadFunc(ctx, localPath, remotePath)
	}
	return nil
}

func (m *MockProvider) Download(ctx context.Context, remotePath, localPath string) error {
	if m.DownloadFunc != nil {
		return m.DownloadFunc(ctx, remotePath, localPath)
	}
	return nil
}

func (m *MockProvider) GetPathID(ctx context.Context, path string, createIfMissing bool) (string, error) {
	if m.GetPathIDFunc != nil {
		return m.GetPathIDFunc(ctx, path, createIfMissing)
	}
	return "", nil
}

func TestPool_ProcessTask_Success(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	// Insert task
	_, err := db.Exec(`
		INSERT INTO tasks (task_id, type, local_path, remote_path, status)
		VALUES ('task-id-123', 'upload', '/local/file.txt', '/remote/file.txt', 'running')
	`)
	if err != nil {
		t.Fatalf("failed to insert test task: %v", err)
	}

	uploadCalled := false
	var uploadMu sync.Mutex
	var calledLocal, calledRemote string

	mockProvider := &MockProvider{
		UploadFunc: func(ctx context.Context, localPath, remotePath string) error {
			uploadMu.Lock()
			defer uploadMu.Unlock()
			uploadCalled = true
			calledLocal = localPath
			calledRemote = remotePath
			return nil
		},
	}

	taskChan := make(chan *queue.Task, 5)
	pool := NewPool(db, mockProvider, taskChan, 1, 100, 3)

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)

	// Send task to worker
	taskChan <- &queue.Task{
		ID:         "task-id-123",
		Type:       "upload",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		Status:     "running",
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	cancel()
	pool.Wait()

	uploadMu.Lock()
	if !uploadCalled {
		t.Error("expected Upload to be called by worker")
	}
	if calledLocal != "/local/file.txt" || calledRemote != "/remote/file.txt" {
		t.Errorf("Upload called with wrong paths: got %s -> %s", calledLocal, calledRemote)
	}
	uploadMu.Unlock()

	// Check status in DB is 'done'
	var status string
	err = db.QueryRow("SELECT status FROM tasks WHERE task_id = 'task-id-123'").Scan(&status)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}
	if status != "done" {
		t.Errorf("expected status 'done', got '%s'", status)
	}
}

func TestPool_ProcessTask_FailAndNoMoreRetries(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	// Insert task that has reached retry limit
	_, err := db.Exec(`
		INSERT INTO tasks (task_id, type, local_path, remote_path, status, retry_count)
		VALUES ('task-id-fail', 'upload', '/local/file.txt', '/remote/file.txt', 'running', 2)
	`)
	if err != nil {
		t.Fatalf("failed to insert test task: %v", err)
	}

	mockProvider := &MockProvider{
		UploadFunc: func(ctx context.Context, localPath, remotePath string) error {
			return errors.New("auth failure")
		},
	}

	taskChan := make(chan *queue.Task, 5)
	// maxRetries = 2, so at retry_count = 2 it fails immediately
	pool := NewPool(db, mockProvider, taskChan, 1, 100, 2)

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)

	taskChan <- &queue.Task{
		ID:         "task-id-fail",
		Type:       "upload",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		Status:     "running",
		RetryCount: 2,
	}

	time.Sleep(100 * time.Millisecond)
	cancel()
	pool.Wait()

	// Check status in DB is 'failed' with error_msg
	var status, errMsg string
	err = db.QueryRow("SELECT status, error_msg FROM tasks WHERE task_id = 'task-id-fail'").Scan(&status, &errMsg)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected status 'failed', got '%s'", status)
	}
	if errMsg != "auth failure" {
		t.Errorf("expected error_msg 'auth failure', got '%s'", errMsg)
	}
}
