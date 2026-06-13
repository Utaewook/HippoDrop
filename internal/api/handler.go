package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"tardis/internal/queue"
)

type Handler struct {
	db *queue.DB
}

func NewHandler(db *queue.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /upload", h.handleUpload)
	mux.HandleFunc("POST /download", h.handleDownload)
	mux.HandleFunc("GET /tasks/{task_id}", h.handleGetTask)
}

type TaskRequest struct {
	LocalPath  string `json:"local_path"`
	RemotePath string `json:"remote_path"`
}

type TaskResponse struct {
	TaskID string `json:"task_id"`
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	h.createTask(w, r, "upload")
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	h.createTask(w, r, "download")
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request, taskType string) {
	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.LocalPath == "" || req.RemotePath == "" {
		http.Error(w, "local_path and remote_path are required", http.StatusBadRequest)
		return
	}

	taskID := uuid.New().String()
	
	query := `INSERT INTO tasks (task_id, type, local_path, remote_path, status, created_at, updated_at) 
	          VALUES (?, ?, ?, ?, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`
	
	_, err := h.db.ExecContext(r.Context(), query, taskID, taskType, req.LocalPath, req.RemotePath)
	if err != nil {
		http.Error(w, "failed to enqueue task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted
	json.NewEncoder(w).Encode(TaskResponse{TaskID: taskID})
}

type TaskStatus struct {
	TaskID     string    `json:"task_id"`
	Type       string    `json:"type"`
	LocalPath  string    `json:"local_path"`
	RemotePath string    `json:"remote_path"`
	Status     string    `json:"status"`
	RetryCount int       `json:"retry_count"`
	ErrorMsg   *string   `json:"error_msg,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *Handler) handleGetTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	query := `SELECT type, local_path, remote_path, status, retry_count, error_msg, created_at FROM tasks WHERE task_id = ?`
	
	var ts TaskStatus
	ts.TaskID = taskID
	
	err := h.db.QueryRowContext(r.Context(), query, taskID).Scan(
		&ts.Type, &ts.LocalPath, &ts.RemotePath, &ts.Status, &ts.RetryCount, &ts.ErrorMsg, &ts.CreatedAt,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to fetch task status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ts)
}
