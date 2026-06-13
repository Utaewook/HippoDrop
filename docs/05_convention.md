# Git & General Convention

This document outlines the Git workflow, commit message format, and general conventions for the Tardis project.

---

## 1. Branching Strategy

- **Strict Branch Limitation:** Only `main` and `develop` branches are permitted for active development.
  - No `feature/*`, `bugfix/*`, or `hotfix/*` branches unless explicitly authorized.
- **`main` Branch:** Reserved exclusively for finalized, production-ready code. Direct commits to `main` are strictly forbidden. Code is merged into `main` only when a release is confirmed.
- **`develop` Branch:** All active development, code revisions, and feature additions are conducted on this branch.
- **Merge Policy:** When development on `develop` reaches a stable release point, a Pull Request (or direct merge if authorized) is created to merge changes into `main`.

---

## 2. Commit Message Format (Conventional Commits)

All commits must strictly adhere to the [Conventional Commits](https://www.conventionalcommits.org/) specification. This ensures a readable history and facilitates automated versioning and changelog generation.

### Format

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Types

- **`feat`**: A new feature implementation.
- **`fix`**: A bug fix.
- **`docs`**: Documentation-only changes.
- **`style`**: Changes that do not affect the meaning of the code (white-space, formatting, missing semi-colons, etc).
- **`refactor`**: A code change that neither fixes a bug nor adds a feature.
- **`perf`**: A code change that improves performance.
- **`test`**: Adding missing tests or correcting existing tests.
- **`chore`**: Changes to the build process or auxiliary tools and libraries such as documentation generation.

### Rules

1. **Description (Subject Line):**
   - Use the imperative, present tense: "change" not "changed" nor "changes".
   - Do not capitalize the first letter.
   - No dot (.) at the end.
   - Keep it concise (under 50 characters if possible).

2. **Body (Optional):**
   - Provide a detailed explanation of the change.
   - Explain the motivation ("why") behind the change, not just the "how".
   - Wrap text at 72 characters.
   - Separate the subject and body with a single blank line.

### Examples

**Good (feat):**
```
feat(api): add status query endpoint for tasks

Adds GET /tasks/{task_id} to allow clients to poll the current
status of an upload or download task from the SQLite queue.
```

**Good (fix):**
```
fix(worker): reset task status on abrupt shutdown

Ensures that any tasks marked as 'running' when a SIGTERM
is received are reset to 'pending' during the graceful
shutdown sequence, preventing permanent task loss.
```

**Good (docs):**
```
docs: update architecture diagram with scheduler details
```

**Bad:**
```
Fixed the worker bug
```
*(Reason: Missing type, past tense, missing context in body)*

---

## 3. Code Reviews

- All code merged into `main` should theoretically pass a review process (even if simulated by an AI assistant).
- The review must check against the [Core Design Constraints](./01_project_overview.md#2-core-design-constraints-non-negotiable) and the [Go Coding Convention](./04_go_convention.md).
- Any violation of the rules (e.g., introducing an unauthorized dependency) must result in rejection.

---

## 4. Next Documents

- [RULES.md — Session Rules](../RULES.md)
- [Project Overview (01_project_overview.md)](./01_project_overview.md)
- [Terminology & Concepts (02_terminology.md)](./02_terminology.md)
- [Architecture & Data Flow (03_architecture.md)](./03_architecture.md)
- [Go Coding Convention (04_go_convention.md)](./04_go_convention.md)
