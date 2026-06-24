# HippoDrop — Session Rules & Checklist

This document defines the mandatory rules and checklists that **all developers and AI assistants** must read and follow at the **start and end of every session**. No exceptions.

---

## 1. Session Start Checklist (Follow in Order)

### Step 1: Verify Git State

1. **Branch & Sync** — Always work on `develop`. Never directly commit to `main`.
   ```bash
   git status
   git pull origin develop
   ```
2. **Review Recent Commits** — Understand what changed in the last session.
   ```bash
   git log -n 5 --oneline
   ```
3. **Clean Workspace** — Confirm no leftover temp files or orphan SQLite locks from previous sessions.

### Step 2: Verify Environment

1. **Go Development Environment** — Use the **Docker container (`hippodrop-dev`)** environment. The source code is mounted to `/app` inside the container. Do NOT run builds or tests directly on the host machine. Build and run it using `./build/docker/run-dev.sh`.
2. **Go toolchain** — Confirm `go version` reports Go 1.25.11 inside the container.
   - Build: `docker exec -w /app hippodrop-dev make build`
   - Test: `docker exec -w /app hippodrop-dev go test ./...`
3. **Config file** — Confirm config file (e.g., config for daemon) exists at the expected path.
4. **Port check** — Default port is **8080**. Confirm nothing else is bound to it inside/outside the container.

### Step 3: Core Constraints Self-Checklist

Before writing or merging any code, verify every item:

- [ ] **No unauthorized dependencies** — Do NOT run `go get` or modify `go.mod/go.sum` unless during initial setup or **explicitly requested by the user**.
- [ ] **Single binary rule** — All logic must remain inside the single HippoDrop process. No external broker (Redis, Celery, RabbitMQ, etc.) may be introduced.
- [ ] **SQLite queue is source-of-truth** — Worker state must always be persisted to SQLite, not held only in memory.
- [ ] **Graceful shutdown** — Any worker change must preserve the `SIGTERM → drain → flush → exit` sequence.
- [ ] **Rate Limiter always active** — The Token Bucket rate limiter must never be bypassed, even during testing.
- [ ] **Ambiguity check** — If any design point is unclear or contested, **stop and invoke `/grill-me`** before proceeding.

---

## 2. Session End Checklist

1. **Clean workspace** — Remove any temp files (e.g., `*.tmp`, test SQLite DBs) from the repo.
2. **Branch check** — Commits must be on `develop`. Direct commits to `main` are **forbidden**.
3. **Commit message** — Strictly follow Conventional Commits format (see [CONTRIBUTING.md](./CONTRIBUTING.md)).
4. **Docs updated** — If terminology, architecture, or constraints changed, update the relevant doc before closing.
5. **Troubleshooting documented** — Ensure any complex bugs fixed or architectural decisions made during the session have been documented via the `/trouble-shooting` skill.

---

## 3. AI Assistant Specific Rules (Mandatory)

- **Thinking language:** All reasoning and planning must be done in **English**.
- **Response language:** Replies to the user must be in **Korean** in a polite and clear structured format.
- **Grill before building:** When any design decision is ambiguous or contested, the AI **must** invoke the `/grill-me` process to resolve it before writing code.
- **No unauthorized installs:** Never run `go get`, `npm install`, `pip install`, or equivalent unless in initial setup or explicitly user-approved.
- **Terminology lock:** Always use the exact terms defined in the architecture and domain docs ([docs/architecture.md](./docs/architecture.md)). Never invent synonyms.
- **Troubleshooting records:** The AI must proactively use the `trouble-shooting` skill to document errors, problems, requirements, and architectural decisions when resolving complex issues.
- **Atomic Commits (Commit Granularity):** Never bundle multiple distinct features, bug fixes, or refactorings into a single git commit. Every independent feature or change must be staged and committed as its own atomic, self-contained commit.
- **Git History Honesty & Transparency:** If you accidentally bundle changes or make a logical error, you must immediately and transparently disclose it to the user. Never perform `git reset`, force-pushes (`git push -f`), or edit git history to hide or gloss over mistakes without explaining the exact situation and obtaining permission first.
- **Strict Branch Constraints:** All development must be done on the `develop` branch. Direct commits to `main` are strictly forbidden. Merges to `main` must never be done automatically; they are permitted ONLY when the user explicitly requests to merge.

---

## 4. When to Invoke `/grill-me`

Use `/grill-me` (or equivalent grill-me skill) whenever:

- A new subsystem or component is being designed for the first time.
- The scope of a feature is ambiguous (e.g., "what counts as a failed task?").
- Two valid implementation approaches exist with significant trade-offs.
- A change would affect the SQLite schema, the `StorageProvider` interface, or the public HTTP API contract.
- The user's intent cannot be unambiguously inferred from the current docs.

> [!IMPORTANT]
> **Never skip the grill step.** Writing code before resolving ambiguity is the primary source of rework in this project.

---

## 5. Reference Documents

- [Architecture & Data Flow (architecture.md)](./docs/architecture.md)
- [Storage Provider (storage.md)](./docs/storage.md)
- [Queue & Worker (queue.md)](./docs/queue.md)
- [HTTP API (api.md)](./docs/api.md)
- [Git & Commit Convention (CONTRIBUTING.md)](./CONTRIBUTING.md)
