package worker

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookSender_SendSuccess(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	apiKey := "my-secret-key"
	receivedPayload := make(chan []byte, 1)
	receivedSignature := make(chan string, 1)

	// Create test webhook receiver server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected method POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type, got %s", r.Header.Get("Content-Type"))
		}

		sig := r.Header.Get("X-Tardis-Signature")
		receivedSignature <- sig

		body, _ := io.ReadAll(r.Body)
		receivedPayload <- body

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 1. Insert a task with pending webhook and 'done' status
	_, err := db.Exec(`
		INSERT INTO tasks (task_id, type, local_path, remote_path, status, callback_url, callback_status)
		VALUES ('task-web-123', 'upload', '/local/1', '/remote/1', 'done', ?, 'pending')
	`, server.URL)
	if err != nil {
		t.Fatalf("failed to insert test task: %v", err)
	}

	sender := NewWebhookSender(db, apiKey)
	sender.processPendingWebhooks(context.Background())

	// 2. Verify webhook payload received
	select {
	case bodyBytes := <-receivedPayload:
		var payload WebhookPayload
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}

		if payload.TaskID != "task-web-123" {
			t.Errorf("expected task_id 'task-web-123', got '%s'", payload.TaskID)
		}
		if payload.Status != "done" {
			t.Errorf("expected status 'done', got '%s'", payload.Status)
		}

		// Verify signature
		sig := <-receivedSignature
		mac := hmac.New(sha256.New, []byte(apiKey))
		mac.Write(bodyBytes)
		expectedSig := hex.EncodeToString(mac.Sum(nil))
		if sig != expectedSig {
			t.Errorf("expected signature %s, got %s", expectedSig, sig)
		}
	default:
		t.Fatal("webhook was not received by the server")
	}

	// 3. Verify task DB status updated to 'sent'
	var callbackStatus string
	err = db.QueryRow("SELECT callback_status FROM tasks WHERE task_id = 'task-web-123'").Scan(&callbackStatus)
	if err != nil {
		t.Fatalf("failed to query callback status: %v", err)
	}
	if callbackStatus != "sent" {
		t.Errorf("expected callback_status 'sent', got '%s'", callbackStatus)
	}
}

func TestWebhookSender_SendFailure(t *testing.T) {
	db, teardown := setupTestDB(t)
	defer teardown()

	// 1. Insert a task with pending webhook targeting an invalid server (will return 500)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := db.Exec(`
		INSERT INTO tasks (task_id, type, local_path, remote_path, status, callback_url, callback_status)
		VALUES ('task-fail-123', 'upload', '/local/1', '/remote/1', 'done', ?, 'pending')
	`, server.URL)
	if err != nil {
		t.Fatalf("failed to insert test task: %v", err)
	}

	sender := NewWebhookSender(db, "")
	sender.processPendingWebhooks(context.Background())

	// 2. Verify status is still 'pending' but callback_retry_count incremented and callback_error set
	var callbackStatus, callbackError string
	var callbackRetryCount int
	err = db.QueryRow("SELECT callback_status, callback_error, callback_retry_count FROM tasks WHERE task_id = 'task-fail-123'").Scan(
		&callbackStatus, &callbackError, &callbackRetryCount,
	)
	if err != nil {
		t.Fatalf("failed to query tasks: %v", err)
	}

	if callbackStatus != "pending" {
		t.Errorf("expected callback_status 'pending', got '%s'", callbackStatus)
	}
	if callbackRetryCount != 1 {
		t.Errorf("expected callback_retry_count 1, got %d", callbackRetryCount)
	}
	if callbackError == "" {
		t.Error("expected non-empty callback_error")
	}
}
