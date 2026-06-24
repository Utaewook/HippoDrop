package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestHandler_CreateTask_Success(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	handler := NewHandler(db, nil, "")
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

	handler := NewHandler(db, nil, "")
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

	handler := NewHandler(db, nil, "")
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

	handler := NewHandler(db, nil, "")
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

func TestHandler_Authentication(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	apiKey := "secret-token"
	handler := NewHandler(db, nil, apiKey)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Request without token -> should fail
	reqNoToken := httptest.NewRequest("GET", "/tasks/test-task-123", nil)
	rrNoToken := httptest.NewRecorder()
	mux.ServeHTTP(rrNoToken, reqNoToken)

	if rrNoToken.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for request without token, got %d", rrNoToken.Code)
	}

	// 2. Request with invalid token -> should fail
	reqBadToken := httptest.NewRequest("GET", "/tasks/test-task-123", nil)
	reqBadToken.Header.Set("Authorization", "Bearer wrong-token")
	rrBadToken := httptest.NewRecorder()
	mux.ServeHTTP(rrBadToken, reqBadToken)

	if rrBadToken.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for request with wrong token, got %d", rrBadToken.Code)
	}

	// 3. Request with valid token but non-existent task -> should proceed to handler logic (returning 404 since task doesn't exist, rather than 401)
	reqGoodToken := httptest.NewRequest("GET", "/tasks/test-task-123", nil)
	reqGoodToken.Header.Set("Authorization", "Bearer "+apiKey)
	rrGoodToken := httptest.NewRecorder()
	mux.ServeHTTP(rrGoodToken, reqGoodToken)

	if rrGoodToken.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for authenticated request of non-existent task, got %d", rrGoodToken.Code)
	}
}

func TestHandler_CreateTask_SecurityValidation_PathTraversal(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	handler := NewHandler(db, nil, "")
	server := httptest.NewServer(http.HandlerFunc(handler.handleUpload))
	defer server.Close()

	tests := []struct {
		name       string
		localPath  string
		statusCode int
	}{
		{"Relative path", "relative/path/to/file.txt", http.StatusBadRequest},
		{"Blocked system path /etc", "/etc/passwd", http.StatusForbidden},
		{"Blocked system path /root", "/root/.ssh/id_rsa", http.StatusForbidden},
		{"Blocked system path /proc", "/proc/self/environ", http.StatusForbidden},
		{"Blocked system path /sys", "/sys/class/net", http.StatusForbidden},
		{"Blocked system path /dev", "/dev/null", http.StatusForbidden},
		{"Blocked system path /boot", "/boot/grub/grub.cfg", http.StatusForbidden},
		{"Blocked system path /var/run", "/var/run/docker.sock", http.StatusForbidden},
		{"Blocked system path /usr/sbin", "/usr/sbin/cron", http.StatusForbidden},
		{"Safe absolute path", "/tmp/safe_file.txt", http.StatusAccepted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := TaskRequest{
				LocalPath:  tt.localPath,
				RemotePath: "/remote/file.txt",
			}
			bodyBytes, _ := json.Marshal(reqBody)

			resp, err := http.Post(server.URL, "application/json", bytes.NewBuffer(bodyBytes))
			if err != nil {
				t.Fatalf("failed to make POST request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, resp.StatusCode)
			}
		})
	}
}

func TestHandler_CreateTask_SecurityValidation_SSRF(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	handler := NewHandler(db, nil, "")
	server := httptest.NewServer(http.HandlerFunc(handler.handleUpload))
	defer server.Close()

	tests := []struct {
		name        string
		callbackURL string
		statusCode  int
	}{
		{"Localhost loopback", "http://localhost:8080/callback", http.StatusAccepted},
		{"127.0.0.1 loopback", "http://127.0.0.1/callback", http.StatusAccepted},
		{"Private IP Class A", "http://10.0.0.1/callback", http.StatusAccepted},
		{"Private IP Class B", "http://172.16.0.1/callback", http.StatusAccepted},
		{"Private IP Class C", "http://192.168.1.1/callback", http.StatusAccepted},
		{"Link-local / Cloud Metadata IP", "http://169.254.169.254/callback", http.StatusBadRequest},
		{"Non-HTTP(S) scheme", "ftp://example.com/callback", http.StatusBadRequest},
		{"Public domain", "https://example.com/callback", http.StatusAccepted},
		{"Empty callback url", "", http.StatusAccepted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := TaskRequest{
				LocalPath:   "/tmp/safe_file.txt",
				RemotePath:  "/remote/file.txt",
				CallbackURL: tt.callbackURL,
			}
			bodyBytes, _ := json.Marshal(reqBody)

			resp, err := http.Post(server.URL, "application/json", bytes.NewBuffer(bodyBytes))
			if err != nil {
				t.Fatalf("failed to make POST request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, resp.StatusCode)
			}
		})
	}
}
