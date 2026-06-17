package queue

import (
	"testing"
)

func TestOpen_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Open database in temporary directory
	db, err := Open(tempDir)
	if err != nil {
		t.Fatalf("failed to Open database: %v", err)
	}
	defer db.Close()

	// Verify schema tables exist
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&tableName)
	if err != nil {
		t.Errorf("failed to find 'tasks' table: %v", err)
	}
	if tableName != "tasks" {
		t.Errorf("expected 'tasks' table, got %s", tableName)
	}

	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='path_cache'").Scan(&tableName)
	if err != nil {
		t.Errorf("failed to find 'path_cache' table: %v", err)
	}
	if tableName != "path_cache" {
		t.Errorf("expected 'path_cache' table, got %s", tableName)
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	// Attempt to open at an invalid directory path (since /etc/hosts is a file, creating a subdirectory under it fails)
	_, err := Open("/etc/hosts/invalid_subdir")
	if err == nil {
		t.Fatal("expected failure when opening db in invalid directory path, got nil")
	}
}

func TestRecoverTasks(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Open first time and populate DB
	db, err := Open(tempDir)
	if err != nil {
		t.Fatalf("failed to Open database: %v", err)
	}

	// Insert running and done tasks
	_, err = db.Exec(`
		INSERT INTO tasks (task_id, type, local_path, remote_path, status)
		VALUES 
			('task-1', 'upload', '/local/1', '/remote/1', 'running'),
			('task-2', 'download', '/local/2', '/remote/2', 'done'),
			('task-3', 'upload', '/local/3', '/remote/3', 'pending')
	`)
	if err != nil {
		t.Fatalf("failed to insert test tasks: %v", err)
	}
	db.Close()

	// 2. Open second time, which triggers recoverTasks on startup
	db2, err := Open(tempDir)
	if err != nil {
		t.Fatalf("failed to Re-open database: %v", err)
	}
	defer db2.Close()

	// 3. Verify status values
	var status1, status2, status3 string
	err = db2.QueryRow("SELECT status FROM tasks WHERE task_id = 'task-1'").Scan(&status1)
	if err != nil {
		t.Fatalf("failed to fetch status for task-1: %v", err)
	}
	if status1 != "pending" {
		t.Errorf("expected task-1 status to be reset to 'pending', got '%s'", status1)
	}

	err = db2.QueryRow("SELECT status FROM tasks WHERE task_id = 'task-2'").Scan(&status2)
	if err != nil {
		t.Fatalf("failed to fetch status for task-2: %v", err)
	}
	if status2 != "done" {
		t.Errorf("expected task-2 status to remain 'done', got '%s'", status2)
	}

	err = db2.QueryRow("SELECT status FROM tasks WHERE task_id = 'task-3'").Scan(&status3)
	if err != nil {
		t.Fatalf("failed to fetch status for task-3: %v", err)
	}
	if status3 != "pending" {
		t.Errorf("expected task-3 status to remain 'pending', got '%s'", status3)
	}
}

func TestMigrateSchema_RunsOnExisting(t *testing.T) {
	tempDir := t.TempDir()

	// Open first time to create and migrate database
	db1, err := Open(tempDir)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	db1.Close()

	// Open second time to verify migrations are idempotent and run successfully on existing DB
	db2, err := Open(tempDir)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	db2.Close()
}
