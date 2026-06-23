package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"tardis/internal/queue"
)

type WebhookPayload struct {
	TaskID     string `json:"task_id"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	LocalPath  string `json:"local_path"`
	RemotePath string `json:"remote_path"`
	RetryCount int    `json:"retry_count"`
	ErrorMsg   string `json:"error_msg,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

type WebhookSender struct {
	db     *queue.DB
	apiKey string
	client *http.Client
}

func NewWebhookSender(db *queue.DB, apiKey string) *WebhookSender {
	return &WebhookSender{
		db:     db,
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *WebhookSender) Start(ctx context.Context) {
	log.Println("Webhook sender started")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Webhook sender shutting down...")
			return
		case <-ticker.C:
			s.processPendingWebhooks(ctx)
		}
	}
}

func (s *WebhookSender) processPendingWebhooks(ctx context.Context) {
	// Query for completed/failed tasks with pending webhooks
	query := `SELECT task_id, type, local_path, remote_path, status, retry_count, error_msg, 
	                 COALESCE(callback_url, ''), callback_retry_count, updated_at 
	          FROM tasks 
	          WHERE callback_status = 'pending' AND status IN ('done', 'failed')
	          LIMIT 10`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("Webhook sender failed to query tasks: %v", err)
		return
	}
	defer rows.Close()

	var tasks []*queue.Task
	for rows.Next() {
		t := &queue.Task{}
		var errorMsg *string
		err := rows.Scan(
			&t.ID, &t.Type, &t.LocalPath, &t.RemotePath, &t.Status, &t.RetryCount, &errorMsg,
			&t.CallbackURL, &t.CallbackRetryCount, &t.UpdatedAt,
		)
		if err != nil {
			log.Printf("Webhook sender failed to scan task: %v", err)
			continue
		}
		if errorMsg != nil {
			t.ErrorMsg = *errorMsg
		}
		tasks = append(tasks, t)
	}

	for _, t := range tasks {
		// Calculate exponential backoff
		// Try 0: instantly, Try 1: 10s, Try 2: 20s, Try 3: 40s, Try 4: 80s, Try 5: 160s
		if t.CallbackRetryCount > 0 {
			backoffSec := math.Pow(2, float64(t.CallbackRetryCount-1)) * 10
			if time.Since(t.UpdatedAt) < time.Duration(backoffSec)*time.Second {
				continue // Backoff is active, skip this task
			}
		}

		s.sendWebhook(ctx, t)
	}
}

func (s *WebhookSender) sendWebhook(ctx context.Context, t *queue.Task) {
	payload := WebhookPayload{
		TaskID:     t.ID,
		Type:       t.Type,
		Status:     t.Status,
		LocalPath:  t.LocalPath,
		RemotePath: t.RemotePath,
		RetryCount: t.RetryCount,
		ErrorMsg:   t.ErrorMsg,
		UpdatedAt:  t.UpdatedAt.Format(time.RFC3339),
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal webhook payload for task %s: %v", t.ID, err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.CallbackURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Printf("Failed to create webhook request for task %s: %v", t.ID, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Tardis-Webhook/1.0")

	// Calculate and set X-Tardis-Signature if API key is set
	if s.apiKey != "" {
		signature := s.computeHMAC(bodyBytes, s.apiKey)
		req.Header.Set("X-Tardis-Signature", signature)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		s.handleFailure(ctx, t, fmt.Sprintf("Network error: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.handleFailure(ctx, t, fmt.Sprintf("HTTP status %d", resp.StatusCode))
		return
	}

	// Success
	s.handleSuccess(ctx, t.ID)
}

func (s *WebhookSender) handleSuccess(ctx context.Context, taskID string) {
	query := `UPDATE tasks 
	          SET callback_status = 'sent', callback_error = NULL, updated_at = CURRENT_TIMESTAMP 
	          WHERE task_id = ?`
	_, err := s.db.ExecContext(ctx, query, taskID)
	if err != nil {
		log.Printf("Failed to update successful webhook status for task %s: %v", taskID, err)
	} else {
		log.Printf("Webhook successfully sent for task %s", taskID)
	}
}

func (s *WebhookSender) handleFailure(ctx context.Context, t *queue.Task, errorMsg string) {
	newRetryCount := t.CallbackRetryCount + 1
	status := "pending"
	if newRetryCount >= 5 {
		status = "failed"
		log.Printf("Webhook delivery permanently failed for task %s after %d retries: %s", t.ID, newRetryCount, errorMsg)
	} else {
		log.Printf("Webhook delivery failed for task %s (attempt %d/5): %s", t.ID, newRetryCount, errorMsg)
	}

	query := `UPDATE tasks 
	          SET callback_status = ?, callback_error = ?, callback_retry_count = ?, updated_at = CURRENT_TIMESTAMP 
	          WHERE task_id = ?`
	_, err := s.db.ExecContext(ctx, query, status, errorMsg, newRetryCount, t.ID)
	if err != nil {
		log.Printf("Failed to update failed webhook status for task %s: %v", t.ID, err)
	}
}

func (s *WebhookSender) computeHMAC(message []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}
