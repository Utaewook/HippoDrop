# Terminology & Concepts

This document is the **single source of truth** for all domain terms used in the Tardis project. Developers and AI assistants must use these exact terms in code, comments, commit messages, and documentation. **Do not invent synonyms.**

> [!IMPORTANT]
> If a term is missing from this dictionary, stop and add it here (with a `/grill-me` session if the definition is ambiguous) before using it anywhere else.

---

## Core Domain Terms

### Task
A discrete unit of work registered in the internal SQLite queue. A Task represents a single upload or download operation and has a lifecycle managed by the Scheduler and Worker Pool.

- **States:** `pending` → `running` → `done` | `failed`
- **Key fields:** `task_id` (UUID), `type` (upload/download), `local_path`, `remote_path`, `status`, `retry_count`, `created_at`, `updated_at`
- **Constraint:** A Task must be written to SQLite **before** the HTTP server returns `202 Accepted`. A Task that only exists in memory is invalid.

---

### Task Queue
The logical queue of Tasks stored in the `tasks` table of the embedded SQLite database. The Task Queue is the authoritative record of all pending and in-progress work.

- **Not** an in-memory channel. The SQLite table is the queue.
- Workers pull Tasks from the Task Queue by querying for `status = 'pending'`.

---

### Worker Pool
The fixed set of Goroutines (count = `workers.pool_size` from config) that continuously pull Tasks from the Task Queue and execute them against the Storage Provider.

- Workers are long-lived; they are not spawned per-task.
- Worker count is set at startup and does not change at runtime.

Workers consume Tasks from a Go Channel fed by the Scheduler.

---

### Provider (Storage Provider)
The abstraction layer (`Provider` Go interface in `internal/storage/provider.go`) that isolates all cloud-specific communication from the Worker Pool. Defines the canonical operations:

| Method | Description |
|---|---|
| `Upload(ctx, localPath, remotePath)` | Transfer a local file to cloud storage |
| `Download(ctx, remotePath, localPath)` | Fetch a remote file to a local path |
| `GetPathID(ctx, path, createIfMissing)` | Resolve a human-readable path string to a cloud-native object ID. Create missing folders if `createIfMissing` is true. |

- **Initial implementation:** `GoogleDriveAdapter`
- **Constraint:** No Worker Pool code may import or reference a concrete adapter directly. Only the `Provider` interface may be used.

---

### GoogleDriveAdapter
The concrete implementation of `Provider` for Google Drive. Responsibilities:

- Manage OAuth2 Refresh Token → Access Token renewal.
- Apply chunked upload for large files (`chunk_size_mb` from config).
- Translate local path strings (e.g., `/Backup/Logs`) to Google Drive folder/file IDs using the Path Cache.

---

### Path Cache
The `path_cache` table in the embedded SQLite database. Stores the mapping between human-readable remote path strings and cloud-native object IDs (e.g., Google Drive folder IDs).

- Purpose: Avoid redundant API traversal calls to resolve the same path repeatedly.
- Owned by: `GoogleDriveAdapter` (or future adapters).
- **Constraint:** The Path Cache is advisory. If a cache entry is stale, the adapter must refresh it via a live API call and update the cache.

---

### Rate Limiter
The Token Bucket algorithm implementation embedded in the Worker Pool. Enforces the `rate_limit_per_second` limit defined in `tardis.yml` for all outbound API calls to the Storage Provider.

- **Constraint:** The Rate Limiter is **always active**. It cannot be disabled or bypassed, including in test environments. Use mock Storage Providers in tests instead.

---

### Scheduler
The internal component responsible for polling the Task Queue and dispatching Tasks to available Workers. The Scheduler is not a separate process — it runs as a Goroutine within the single Tardis binary.

The Scheduler polls the Task Queue (1-second interval) and pushes Tasks to a Go Channel for Workers to consume. Before dispatching, it updates the Task status to `running` using optimistic locking (`WHERE status = 'pending'`) to prevent duplicate dispatch.

---

### Graceful Shutdown
The shutdown sequence triggered on `SIGTERM`:

1. Stop accepting new Tasks from the HTTP API.
2. Allow in-progress Worker tasks to complete their current chunk transfer.
3. Flush all Task states to SQLite.
4. Close the SQLite connection cleanly.
5. Exit the process.

- **Constraint:** The process must not exit until step 4 is complete. Abrupt shutdown that leaves Tasks in `running` state without resetting them to `pending` is a bug.

---

### Chunk
A fixed-size slice of a large file, used during upload or download. Size is controlled by `workers.chunk_size_mb` in `tardis.yml`.

- Chunking is the responsibility of the Storage Provider adapter, not the Worker Pool.

---

### tardis.yml
The primary configuration file for Tardis. Loaded at startup via the `-c` flag. Controls all runtime parameters: server port, data directory, storage provider credentials, rate limits, worker pool size, and chunk size.

---

### data_dir
The local filesystem directory where Tardis stores its embedded SQLite database and any temporary files. Defined in `tardis.yml` under `server.data_dir`. Must be writable by the Tardis process.

---

## Resolved Design Decisions

The following design questions have been resolved during implementation.

| Term | Resolution |
|---|---|
| **Worker dispatch model** | Scheduler polls SQLite → pushes to Go Channel for Workers. (Resolved) |
| **SQLite driver** | `github.com/mattn/go-sqlite3` (CGO). (Resolved) |
| **Retry state reset** | Yes — on startup, all `running` tasks are automatically reset to `pending`. See `recoverTasks()` in `internal/queue/db.go`. (Resolved) |
| **Prefetch API** | `POST /download` with `{"remote_path": "...", "local_path": "..."}` → `202 Accepted` + `task_id`. (Resolved) |
| **Status query API** | `GET /tasks/{task_id}` returns full metadata: `task_id`, `type`, `local_path`, `remote_path`, `status`, `retry_count`, `error_msg`, `created_at`. (Resolved) |

---

## Next Documents

- [RULES.md — Session Rules](../RULES.md)
- [Project Overview (01_project_overview.md)](./01_project_overview.md)
- [Architecture & Data Flow (03_architecture.md)](./03_architecture.md)
- [Go Coding Convention (04_go_convention.md)](./04_go_convention.md)
- [Git & General Convention (05_convention.md)](./05_convention.md)
