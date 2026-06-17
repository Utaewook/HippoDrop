package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tardis/internal/queue"
)

func setupTestDB(t *testing.T) (*queue.DB, func()) {
	tempDir := t.TempDir()
	db, err := queue.Open(tempDir)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return db, func() { db.Close() }
}

func TestHandler_CreateTask_Success(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	handler := NewHandler(db, nil)
	server := httptest.NewServer(http.HandlerFunc(handler.handleUpload))
	defer server.Close()

	reqBody := TaskRequest{
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("failed to make POST request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected status 202 Accepted, got %d", resp.StatusCode)
	}

	var taskResp TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if taskResp.TaskID == "" {
		t.Error("expected non-empty task_id in response")
	}

	// Verify it was written to database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE task_id = ?", taskResp.TaskID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query database for task: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 task in database, got %d", count)
	}
}

func TestHandler_CreateTask_MissingFields(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	handler := NewHandler(db, nil)
	server := httptest.NewServer(http.HandlerFunc(handler.handleUpload))
	defer server.Close()

	// Missing RemotePath
	reqBody := TaskRequest{
		LocalPath: "/local/file.txt",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("failed to make POST request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestHandler_CreateTask_InvalidJSON(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	handler := NewHandler(db, nil)
	server := httptest.NewServer(http.HandlerFunc(handler.handleUpload))
	defer server.Close()

	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString("{invalid-json"))
	if err != nil {
		t.Fatalf("failed to make POST request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestHandler_GetTask(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	// Insert a task directly to DB
	_, err := db.Exec(`
		INSERT INTO tasks (task_id, type, local_path, remote_path, status)
		VALUES ('test-task-123', 'download', '/local/path', '/remote/path', 'pending')
	`)
	if err != nil {
		t.Fatalf("failed to insert test task: %v", err)
	}

	handler := NewHandler(db, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test GET for existing task
	req := httptest.NewRequest("GET", "/tasks/test-task-123", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", rr.Code)
	}

	var status TaskStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to unmarshal task status: %v", err)
	}

	if status.TaskID != "test-task-123" {
		t.Errorf("expected task_id 'test-task-123', got '%s'", status.TaskID)
	}
	if status.Type != "download" {
		t.Errorf("expected type 'download', got '%s'", status.Type)
	}
	if status.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", status.Status)
	}

	// Test GET for non-existing task
	reqNotFound := httptest.NewRequest("GET", "/tasks/unknown-task", nil)
	rrNotFound := httptest.NewRecorder()
	mux.ServeHTTP(rrNotFound, reqNotFound)

	if rrNotFound.Code != http.StatusNotFound {
		t.Errorf("expected status 404 Not Found, got %d", rrNotFound.Code)
	}
}
