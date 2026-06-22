# Tardis Architecture Decisions (Grill-Me Session)

- **Date:** 2026-06-13
- **Type:** Decision

## Context
During the initial implementation planning phase for Tardis, several architectural design choices regarding the Scheduler, Task Recovery, API Schema, and File Download mechanisms were left open.

## Issue/Requirement
Before writing the core logic, we needed to lock in specific behaviors to ensure consistency and avoid rework, particularly around:
1. How the Scheduler dispatches tasks to Workers.
2. How to handle interrupted tasks on server restart.
3. The exact response schema for task status polling.
4. The exact request/response format for the prefetch/download API.
5. How downloads are transferred (chunked vs direct stream).

## Investigation
A `/grill-me` session was conducted with the user to walk through the design tree and evaluate trade-offs for each component.

## Resolution
The following decisions were made:

1. **Scheduler Dispatch:** The Scheduler will poll SQLite for pending tasks and push them to a Go Channel for Workers to consume. This is more idiomatic to Go and reduces SQLite lock contention compared to having N workers polling the DB.
2. **Startup Recovery:** Any tasks left in the `running` state upon unexpected shutdown will automatically be rolled back to `pending` on startup to ensure no tasks are permanently lost.
3. **Status Endpoint Schema:** `GET /tasks/{task_id}` will return detailed metadata (`type`, `local_path`, `remote_path`, `retry_count`, `created_at`) rather than a minimalist schema.
4. **Prefetch API Contract:** The download API will mirror the upload API (`POST /download` with `{remote_path, local_path}`), returning a `202 Accepted` and `task_id`.
5. **Download Mechanism:** To support large files safely and allow partial retries, downloads will use HTTP Range requests for Chunk-based resumable transfers, matching the upload strategy.

Documents `docs/architecture.md` and `CLAUDE.md` have been updated to reflect these decisions.
