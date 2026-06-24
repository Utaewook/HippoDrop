# Contributing to Tardis

## Git Commit Convention

Tardis strictly follows the **[Conventional Commits](https://www.conventionalcommits.org/)** specification. This ensures a readable project history and enables automated semantic versioning and changelog generation.

### Commit Message Format
```
<type>(<scope>): <subject>
```
*(Note: `(<scope>)` is optional but highly recommended for clear tracking).*

### 1. Types (`<type>`)
- **`feat`**: Adds a new feature or capability.
- **`fix`**: Fixes a bug.
- **`docs`**: Documentation only changes (e.g., `README.md`, `docs/`).
- **`refactor`**: A code change that neither fixes a bug nor adds a feature (e.g., restructuring).
- **`test`**: Adding missing tests or correcting existing tests.
- **`ci`**: Changes to CI configuration files and scripts (e.g., GitHub Actions, GoReleaser).
- **`chore`**: Maintenance tasks, build processes, auxiliary tools, or `.gitignore` updates.

### 2. Scopes (`<scope>`)
Use scopes to indicate the specific module or component being modified:
- **`cli`**: CLI commands, flags, argument parsing.
- **`tui`**: The interactive terminal UI wizard (`huh` forms, guides).
- **`core`**: Main application assembly, graceful shutdown, orchestration.
- **`api`**: HTTP routes and handlers (`/upload`, `/download`, `/status`).
- **`worker`**: Background worker pool, rate limiters, scheduler.
- **`storage`**: Storage provider adapters (e.g., cloud storage logic).
- **`queue`**: SQLite database, task schema, state recovery.
- **`config`**: YAML parsing, project settings.
- **`install`**: Installation and uninstallation scripts (`install.sh`, `uninstall.sh`).

### 3. Subject (`<subject>`)
- Use the **imperative, present tense**: "change" not "changed" nor "changes".
- Do not capitalize the first letter.
- Do not put a period (`.`) at the end.

---

### Examples of Good Commits

- `feat(tui): support ESC key to quit the init wizard`
- `fix(storage): prevent path traversal in remote path resolution`
- `refactor(tui): isolate GCP guide into its own internal/assets package`
- `docs(tui): improve cloud storage setup guide description`
- `ci: transition release pipeline to GoReleaser and CGO docker build`
- `chore: add Makefile for build and system-wide installation`

---

## Branching Strategy

Tardis follows a simplified Git Flow model using two primary branches:

1. **`main`**: The stable, production-ready branch. All commits here must be releasable.
2. **`develop`**: The active development branch. All new features, refactoring, and bug fixes are committed or merged here first.

**Workflow**:
- Work on features locally or in feature branches (`feature/<name>`).
- Merge features into `develop`.
- When a set of features is ready for release, merge `develop` into `main`.

---

## Tagging and Release Strategy

Releases are fully automated via GitHub Actions (`.github/workflows/release.yml`) using **GoReleaser**, but strict rules apply to tags:

1. **Tag Format**: Must use Semantic Versioning starting with `v` (e.g., `v1.0.0`, `v0.1.0-beta.6`).
2. **Branch Restriction**: Tags **MUST be created on the `main` branch**. 
   - *Why?* The CI pipeline enforces `git merge-base --is-ancestor $TAG_COMMIT main`. If a tag is pushed from `develop` or a feature branch, the release build will intentionally fail.
3. **Trigger**: Pushing the tag to the remote (`git push origin <tag>`) automatically triggers the CI to cross-compile the binaries and draft a GitHub Release.
