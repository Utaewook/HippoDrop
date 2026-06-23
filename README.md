<p align="center">
  <h1 align="center">🚀 Tardis</h1>
  <p align="center">
    <strong>A lightweight, asynchronous cloud storage proxy daemon written in Go</strong>
  </p>
  <p align="center">
    <a href="https://github.com/Utaewook/Tardis/releases/latest"><img src="https://img.shields.io/github/v/release/Utaewook/Tardis?style=flat-square&color=blue" alt="Latest Release"></a>
    <a href="https://github.com/Utaewook/Tardis/actions"><img src="https://img.shields.io/github/actions/workflow/status/Utaewook/Tardis/release.yml?style=flat-square" alt="Build Status"></a>
    <a href="https://goreportcard.com/report/github.com/Utaewook/Tardis"><img src="https://goreportcard.com/badge/github.com/Utaewook/Tardis?style=flat-square" alt="Go Report Card"></a>
    <a href="./LICENSE"><img src="https://img.shields.io/github/license/Utaewook/Tardis?style=flat-square" alt="License"></a>
    <a href="https://ko-fi.com/twyou"><img src="https://img.shields.io/badge/Ko--fi-F16061?style=flat-square&logo=ko-fi&logoColor=white" alt="Ko-fi"></a>
  </p>
</p>

---

Tardis sits between your local applications and cloud storage providers, handling file uploads and downloads **asynchronously in the background**. Send a fire-and-forget HTTP request, get a `task_id` back instantly, and let Tardis handle the rest.

```
Your App ──HTTP──▶ Tardis ──async──▶ Google Drive
                     │
              SQLite Queue
         (zero state loss guarantee)
```

## 📥 Installation

### Quick Install (Linux / macOS)

```bash
curl -sSL https://raw.githubusercontent.com/Utaewook/Tardis/main/scripts/install.sh | sh
```

This script automatically detects your OS and architecture, downloads the correct binary, and installs it to `/usr/local/bin/tardis`.

### Uninstall

To completely remove Tardis (binary, system configuration, and database) from your system, run:

```bash
curl -sSL https://raw.githubusercontent.com/Utaewook/Tardis/main/scripts/uninstall.sh | sh
```

### Manual Download

Download the binary for your platform from the [latest release](https://github.com/Utaewook/Tardis/releases/latest):

| Platform | Architecture | Download |
|---|---|---|
| Linux | x86_64 | [`tardis-linux-amd64`](https://github.com/Utaewook/Tardis/releases/latest/download/tardis-linux-amd64) |
| Linux | ARM64 | [`tardis-linux-arm64`](https://github.com/Utaewook/Tardis/releases/latest/download/tardis-linux-arm64) |
| macOS | Apple Silicon | [`tardis-darwin-arm64`](https://github.com/Utaewook/Tardis/releases/latest/download/tardis-darwin-arm64) |


### Build from Source

Requires Go 1.25+ and a C compiler (for CGO/SQLite).

```bash
git clone https://github.com/Utaewook/Tardis.git
cd Tardis
make build      # → build/tardis
sudo make install   # → /usr/local/bin/tardis
```

## 🚀 Quick Start

### 1. Initialize Configuration

Run the interactive setup wizard for a new project:

```bash
tardis init my-app
```

This launches a full-screen TUI that guides you through:
- Selecting your cloud storage provider (Google Drive)
- Setting the path to your OAuth Client ID credentials (client_secret.json)
- Choosing a root directory name on Google Drive

Configuration is saved to `~/.tardis/projects/my-app/config.yml`.

### 2. Start the Daemon

```bash
tardis start my-app
```

### 3. Upload a File (with Optional Webhook & Authentication)

```bash
curl -X POST http://localhost:8080/upload \
  -H "Authorization: Bearer <your-secure-token>" \
  -H "Content-Type: application/json" \
  -d '{"local_path": "/path/to/file.txt", "remote_path": "backups/file.txt", "callback_url": "https://your-app.com/webhook"}'
```

Response:
```json
{"task_id": "550e8400-e29b-41d4-a716-446655440000"}
```

### 4. Check Task Status

```bash
curl -H "Authorization: Bearer <your-secure-token>" http://localhost:8080/tasks/550e8400-e29b-41d4-a716-446655440000
```

Response:
```json
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "upload",
  "status": "done",
  "local_path": "/path/to/file.txt",
  "remote_path": "backups/file.txt",
  "retry_count": 0,
  "callback_url": "https://your-app.com/webhook",
  "callback_status": "sent",
  "callback_retry_count": 0,
  "created_at": "2026-06-17T10:00:00Z"
}
```

## 📡 API Reference

> [!NOTE]
> If `server.api_key` is configured in `config.yml`, all endpoints (except `/health`) require `Authorization: Bearer <API_KEY>` header.

| Method | Endpoint | Description | Response |
|---|---|---|---|
| `POST` | `/upload` | Queue a file upload | `202 Accepted` + `{task_id}` |
| `POST` | `/download` | Queue a file download | `202 Accepted` + `{task_id}` |
| `GET` | `/tasks/{task_id}` | Get task status | Task metadata JSON |
| `GET` | `/health` | Health check | `200 OK` |

### Request Body (Upload / Download)

```json
{
  "local_path": "/absolute/path/to/local/file",
  "remote_path": "relative/path/on/cloud/storage",
  "callback_url": "https://optional-callback-webhook.com"
}
```

## 🔧 CLI Reference

```
tardis init <project>         Launch setup wizard for a new project
tardis start <project> [-d]   Start the daemon (use -d for background)
tardis ls [-a] [-l]           List daemons (-a: all, -l: details)
tardis status <project>       Check the daemon running status
tardis stop <project>         Stop the running daemon gracefully
tardis pause <project>        Pause dispatching new tasks
tardis resume <project>       Resume dispatching tasks
tardis rm <project>           Remove a stopped project completely
tardis --version              Show version
tardis --help                 Show help
```

## 🛠️ Task Lifecycle

```
pending ──▶ running ──▶ done
               │
               ├──▶ pending (retry with exponential backoff)
               │
               └──▶ failed  (max retries exceeded)
```

- On startup, any `running` tasks from a previous crash are automatically reset to `pending`.
- Rate limiting is **always active** — even during retries.
- Graceful shutdown completes in-progress chunk transfers before exiting.

## 📋 Supported Platforms

| Platform | Architecture | Status |
|---|---|---|
| Linux | x86_64 (amd64) | ✅ Supported |
| Linux | ARM64 | ✅ Supported |
| macOS | Apple Silicon (arm64) | ✅ Supported |
| Windows | WSL | ✅ Use Linux binary |

## 📜 License

This project is licensed under the terms specified in the [LICENSE](./LICENSE) file.
