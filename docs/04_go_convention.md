# Go Coding Convention (Must Follow)

All Go code written for the Tardis project must strictly adhere to the following rules. These rules prioritize simplicity, correctness, and adherence to standard Go idioms.

---

## 1. General Go Principles

- **Standard library first:** Prefer the Go standard library over third-party packages whenever possible.
- **Go idioms:** Follow the guidelines established in [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).
- **Simplicity:** Write code that is easy to read and understand. Avoid unnecessary abstractions.
- **Documentation:** Use standard Go doc comments (`//`) to document all exported types, functions, methods, and constants. Explain *why* something is done, not just *what* it does.

---

## 2. Dependency Management

- **`go.mod` is sacred:** Do NOT manually edit `go.mod` or `go.sum`. Always use `go get`, `go mod tidy`, and `go mod vendor` to manage dependencies.
- **No unauthorized dependencies:** As stated in the core constraints, you must **never** introduce new third-party dependencies without explicit user permission or unless it is part of the initial agreed-upon setup (e.g., a specific SQLite driver).

---

## 3. Concurrency and Synchronization

- **Goroutines:** Do not spawn unbounded goroutines. Use the configured Worker Pool size (`workers.pool_size`) to limit concurrency.
- **Channels vs. Mutexes:** Use channels to coordinate ownership and pass data between goroutines. Use `sync.Mutex` or `sync.RWMutex` to protect shared state within a single component.
- **Context:** Always pass `context.Context` as the first argument to functions that perform I/O, network requests, or long-running operations. Respect `ctx.Done()` for cancellation and timeouts.
- **WaitGroups:** Use `sync.WaitGroup` to wait for a collection of goroutines to finish (e.g., during graceful shutdown).

---

## 4. Error Handling

- **Explicit returns:** Return errors explicitly as the last return value. Do not use `panic` for normal error control flow.
- **Contextual errors:** When returning an error from a nested function call, wrap it with additional context using `fmt.Errorf("failed to do X: %w", err)`. This preserves the original error for inspection via `errors.Is` or `errors.As`.
- **Handle or return:** Do not ignore errors (e.g., using `_ = func()`). Either handle the error gracefully or return it to the caller.
- **Logging:** Log errors at the appropriate level. Do not log an error and then return it, as this leads to duplicate log entries. Usually, the top-level caller (e.g., the HTTP handler or the top level of a Worker goroutine) should be responsible for logging.

---

## 5. Testing

- **Table-driven tests:** Use table-driven tests for functions with multiple input combinations.
- **Interfaces for mocking:** As designed, use interfaces (like `StorageProvider`) to mock external dependencies in unit tests.
- **No external I/O in unit tests:** Unit tests must not make real network requests or read/write to the real filesystem (outside of temporary test directories). Use mocks or the `httptest` package.
- **Test coverage:** Aim for high test coverage, particularly for core logic like the Scheduler, Worker Pool, and HTTP handlers.

---

## 6. Project Structure (Standard Go Layout)

Follow a simplified version of the Standard Go Project Layout:

```
tardis/
├── cmd/
│   └── tardis/
│       └── main.go       # Application entry point
├── internal/             # Private application code (cannot be imported by other projects)
│   ├── api/              # HTTP handlers and routing
│   ├── config/           # Configuration loading (tardis.yml)
│   ├── queue/            # SQLite Task Queue implementation
│   ├── storage/          # StorageProvider interface and adapters (Google Drive)
│   └── worker/           # Worker Pool and Scheduler logic
├── docs/                 # Documentation (this directory)
├── RULES.md              # Mandatory session rules
├── tardis.example.yml    # Example configuration file
├── go.mod
└── go.sum
```

---

## 7. Next Documents

- [RULES.md — Session Rules](../RULES.md)
- [Project Overview (01_project_overview.md)](./01_project_overview.md)
- [Terminology & Concepts (02_terminology.md)](./02_terminology.md)
- [Architecture & Data Flow (03_architecture.md)](./03_architecture.md)
- [Git & General Convention (05_convention.md)](./05_convention.md)
