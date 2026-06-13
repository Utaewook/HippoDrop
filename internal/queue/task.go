package queue

import "time"

// Task represents a unit of work (upload or download)
type Task struct {
	ID         string
	Type       string // "upload" or "download"
	LocalPath  string
	RemotePath string
	Status     string
	RetryCount int
	ErrorMsg   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
