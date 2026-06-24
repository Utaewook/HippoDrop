package simulator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type Tracker struct {
	targetURL    string
	pollInterval time.Duration
	tasks        map[string]*TaskInfo
	mu           sync.Mutex
	client       *http.Client
	webhookURL   string
}

type TaskInfo struct {
	TaskID    string
	LocalPath string
	CreatedAt time.Time
}

func NewTracker(targetURL string, pollInterval time.Duration, webhookURL string) *Tracker {
	return &Tracker{
		targetURL:    targetURL,
		pollInterval: pollInterval,
		tasks:        make(map[string]*TaskInfo),
		client:       &http.Client{Timeout: 10 * time.Second},
		webhookURL:   webhookURL,
	}
}

func (t *Tracker) AddTask(taskID, localPath string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tasks[taskID] = &TaskInfo{
		TaskID:    taskID,
		LocalPath: localPath,
		CreatedAt: time.Now(),
	}
}

func (t *Tracker) UploadFile(localPath, remotePath string) error {
	payload := map[string]string{
		"local_path":  localPath,
		"remote_path": remotePath,
	}
	if t.webhookURL != "" {
		payload["callback_url"] = t.webhookURL
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", t.targetURL+"/upload", bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var res map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	taskID, ok := res["task_id"]
	if !ok {
		return fmt.Errorf("no task_id returned")
	}

	t.AddTask(taskID, localPath)
	return nil
}

func (t *Tracker) Start() {
	ticker := time.NewTicker(t.pollInterval)
	for range ticker.C {
		t.checkTasks()
	}
}

func (t *Tracker) checkTasks() {
	t.mu.Lock()
	activeTasks := make([]*TaskInfo, 0, len(t.tasks))
	for _, info := range t.tasks {
		activeTasks = append(activeTasks, info)
	}
	t.mu.Unlock()

	for _, info := range activeTasks {
		status, err := t.pollTask(info.TaskID)
		if err != nil {
			log.Printf("[Warning] [Tracker] Error polling task %s: %v", info.TaskID, err)
			continue
		}

		if status == "done" {
			latency := time.Since(info.CreatedAt)
			log.Printf("[Completed] Task %s | Latency: %s | Removed: %s", info.TaskID, latency.Round(time.Millisecond), info.LocalPath)
			
			// Auto delete original file
			_ = os.Remove(info.LocalPath)

			t.mu.Lock()
			delete(t.tasks, info.TaskID)
			t.mu.Unlock()
		} else if status == "failed" {
			log.Printf("[Failed] Task %s | Retained: %s", info.TaskID, info.LocalPath)
			t.mu.Lock()
			delete(t.tasks, info.TaskID)
			t.mu.Unlock()
		}
	}
}

func (t *Tracker) pollTask(taskID string) (string, error) {
	resp, err := t.client.Get(fmt.Sprintf("%s/tasks/%s", t.targetURL, taskID))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status code %d", resp.StatusCode)
	}

	var res struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return res.Status, nil
}

type webhookPayload struct {
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	LocalPath string `json:"local_path"`
}

func (t *Tracker) StartWebhookServer(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", t.handleWebhook)
	
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	
	log.Printf("[Webhook] Listening for completion callbacks on :%d/webhook", port)
	return srv.ListenAndServe()
}

func (t *Tracker) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var p webhookPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))

	t.mu.Lock()
	info, exists := t.tasks[p.TaskID]
	if exists {
		delete(t.tasks, p.TaskID)
	}
	t.mu.Unlock()

	if exists {
		if p.Status == "done" {
			latency := time.Since(info.CreatedAt)
			log.Printf("[Completed] Task %s | Latency: %s | Removed: %s (via webhook)", p.TaskID, latency.Round(time.Millisecond), info.LocalPath)
			_ = os.Remove(info.LocalPath)
		} else if p.Status == "failed" {
			log.Printf("[Failed] Task %s | Retained: %s (via webhook)", p.TaskID, info.LocalPath)
		}
	}
}
