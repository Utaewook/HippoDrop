# Project Overview: Tardis (Storage Gateway Proxy)

## 1. Project Description

**Tardis** is a lightweight, asynchronous storage gateway daemon written in Go. It sits between local applications (AI pipelines, data processing workers, MSA services) and cloud storage providers (initially Google Drive), decoupling the main application from the latency and complexity of cloud I/O.

Tardis ships as a **single standalone binary** — no runtime dependencies, no external broker, no Docker required (though a scratch-based container image is also supported). It behaves like a local HTTP sidecar that accepts fire-and-forget upload/download requests and handles all cloud communication in the background.

---

## 2. Core Design Constraints (Non-Negotiable)

| Constraint | Rule |
|---|---|
| **Single binary** | All logic runs in one Go process. No Redis, Celery, or external brokers allowed. |
| **No memory-only state** | All task state must be persisted to the embedded SQLite database before acknowledging a request. |
| **Fire-and-forget API** | The HTTP server must return `202 Accepted` + `task_id` immediately. It must never block on I/O. |
| **Rate limiting always on** | The Token Bucket rate limiter must be active at all times. It cannot be disabled, even in tests. |
| **Graceful shutdown** | On `SIGTERM`, the daemon must drain in-progress chunk transfers, flush SQLite state, and then exit cleanly. |
| **No unauthorized deps** | Do NOT introduce new Go modules unless during initial setup or with explicit user approval. |

> [!IMPORTANT]
> If any of these constraints seem to conflict with a proposed implementation, **stop and invoke `/grill-me`** before proceeding.

---

## 3. Technology Stack

| Layer | Technology | Notes |
|---|---|---|
| **Language** | Go (Golang) | Cross-compile, static linking, Goroutine-native concurrency |
| **Embedded DB** | SQLite (via `modernc.org/sqlite` or `mattn/go-sqlite3`) | Task queue persistence + path-ID mapping cache |
| **Configuration** | YAML (`tardis.yml`) | Injected at startup via `-c` flag |
| **Deployment** | Native OS binary + `scratch`-based Docker container | No base OS layer in container |
| **Initial Storage Provider** | Google Drive API (OAuth2 Refresh Token flow) | Abstracted behind `StorageProvider` interface |

> [!NOTE]
> The specific SQLite driver (`modernc` vs `mattn`) is an open design decision. See [02_terminology.md](./02_terminology.md) for the trade-off summary. Invoke `/grill-me` if this has not been resolved.

---

## 4. Version Control & Branching

- **Git branching:** `main` (production releases only) → `develop` (all active work).
- **Direct commits to `main` are forbidden.** Merge only when a release is confirmed.
- **Commit format:** Conventional Commits (see [05_convention.md](./05_convention.md)).

---

## 5. Next Documents

- [RULES.md — Session Rules](../RULES.md)
- [Terminology & Concepts (02_terminology.md)](./02_terminology.md)
- [Architecture & Data Flow (03_architecture.md)](./03_architecture.md)
- [Go Coding Convention (04_go_convention.md)](./04_go_convention.md)
- [Git & General Convention (05_convention.md)](./05_convention.md)
