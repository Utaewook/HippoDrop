package queue

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// DB encapsulates the SQLite database connection.
type DB struct {
	*sql.DB
}

// Open connects to the SQLite database and ensures the schema is up to date.
func Open(dataDir string) (*DB, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "tardis.db")
	
	// Open SQLite connection with WAL mode enabled for better concurrency
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite is best used with a single concurrent writer to avoid database is locked issues.

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite db: %w", err)
	}

	if err := migrateSchema(db); err != nil {
		return nil, fmt.Errorf("failed to migrate schema: %w", err)
	}

	// Startup Recovery: Reset any 'running' tasks to 'pending' to prevent orphans after a crash
	if err := recoverTasks(db); err != nil {
		return nil, fmt.Errorf("failed to recover tasks on startup: %w", err)
	}

	return &DB{db}, nil
}

func migrateSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS tasks (
		task_id     TEXT PRIMARY KEY,
		type        TEXT NOT NULL,          -- 'upload' | 'download'
		local_path  TEXT NOT NULL,
		remote_path TEXT NOT NULL,
		status      TEXT NOT NULL DEFAULT 'pending', -- 'pending'|'running'|'done'|'failed'
		retry_count INTEGER NOT NULL DEFAULT 0,
		error_msg   TEXT,
		created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS path_cache (
		path        TEXT PRIMARY KEY,
		object_id   TEXT NOT NULL,
		cached_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`

	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Add callback columns dynamically if they do not exist
	var hasCallbackURL bool
	err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('tasks') WHERE name='callback_url'").Scan(&hasCallbackURL)
	if err == nil && !hasCallbackURL {
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN callback_url TEXT"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN callback_status TEXT NOT NULL DEFAULT 'none'"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN callback_retry_count INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN callback_error TEXT"); err != nil {
			return err
		}
	}
	return nil
}

func recoverTasks(db *sql.DB) error {
	// Any tasks that were 'running' when the server crashed should be reset to 'pending'
	_, err := db.Exec("UPDATE tasks SET status = 'pending', updated_at = CURRENT_TIMESTAMP WHERE status = 'running'")
	return err
}
