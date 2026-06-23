# CLAUDE.md

이 파일은 AI 에이전트(Claude Code 등)가 본 저장소에서 작업할 때 참고하는 **트리거 라우터**입니다.
상세 내용은 담지 않습니다 — "무엇을 건드리면 어떤 문서를 먼저 읽어야 하는가"만 라우팅하고,
개념·계약은 `docs/*.md`, 절차는 `.claude/commands/*.md` 로 위임합니다.
이 분담의 규칙은 `docs/_AUTHORING.md` 가 단일 출처입니다.

## 런타임 및 환경

- Go 1.25+. 소스 루트는 `cmd/`, `internal/` 이며 Go Module(`go.mod`) 규약.
- **개발 및 테스트 환경 (Docker 컨테이너)**: 환경 격리를 위해 `tardis-dev` 컨테이너 내부에서 작업을 진행합니다.
  - 빌드/구동 스크립트: `./build/docker/run-dev.sh`
  - 소스코드 마운트 위치: 컨테이너 내부 `/app`
  - Go 바이너리 경로: `/usr/local/go/bin/go` (Go 1.25.11)
  - 빌드 실행: `docker exec -w /app tardis-dev make build`
  - 테스트 실행: `docker exec -w /app tardis-dev go test ./...`
- 실행 환경: Native 바이너리 (또는 Docker). 빌드·의존성 정의는 `go.mod` / `Makefile` 단일 출처.

## 엔트리포인트 / 노드 실행

- `cmd/tardis/main.go` 가 유일한 시작점. CLI 모드(`init`, `start`, `status` 등)로 동작.
- 실행 인자는 `tardis start -c <config.yml>` 형태. `tardis init`으로 설정 생성.

## 검증/테스트 절차

작업 검증은 **반드시 실제 실행 환경에서** 수행합니다 — 호스트 직접 실행이나 단위 테스트
단독 종료 금지(환경 의존성으로 인한 거짓 통과/실패 방지). 변경 범위에 맞는 슬래시 커맨드를
사용하세요 (절차·헬스 라인·정리까지 자기완결적):

- 단일/다중 컴포넌트 변경 → `/verify` (`.claude/commands/verify.md`)

## 트리거 라우팅 (핵심)

> 아래 경로/파일을 건드리는 작업은 **시작 전 지정된 문서를 먼저 읽고** 계약을 확인할 것.

| 건드리는 대상 (경로/파일) | 먼저 읽을 문서 | 비고 |
| --- | --- | --- |
| `cmd/tardis/**` (시스템 기동/CLI 전반) | `docs/architecture.md` | 싱글 바이너리 제약, 종료 시퀀스 |
| `internal/storage/**` (스토리지 프로바이더) | `docs/storage.md` | 새 backend 는 `/new-storage` |
| `internal/queue/**`, `internal/worker/**` | `docs/queue.md` | 큐/워커풀 동시성, SQLite 영속성 |
| `internal/api/**` (HTTP API) | `docs/api.md` | Fire-and-forget 계약 |
| Git 브랜치, 태그, 커밋 작성 시 | `CONTRIBUTING.md` | 커밋 포맷, 브랜치 전략, 릴리스 규칙 |

## 확장 절차 (Extension Playbooks)

확장 작업은 대부분 **기존 파일 수정 대신 새 파일 추가**로 이루어지도록 설계합니다.
아래 슬래시 커맨드가 계약을 어기지 않는 절차·파일 레이아웃·디스패처 연결·검증까지 수행합니다:

- 새 Storage 백엔드 → `/new-storage` (`.claude/commands/new-storage.md`)

### 계약 파일 (contract files) — 수정 시 신중

다음 파일은 플랫폼 전체의 인터페이스를 정의합니다. **확장이 아니라 플랫폼 변경일 때만** 수정하세요.
(같은 목록을 `.claude/contract_files.txt` 에도 적으면 `contract_guard` 훅이 편집 시 경고합니다.)

- `internal/storage/provider.go` — `Provider` 인터페이스(업로드/다운로드 등). 상세: `docs/storage.md`.

---

## md 파일 수정 — 모듈성 체크 (메타 규칙: 그대로 유지 권장)

본 저장소의 md 시스템은 **CLAUDE.md = 트리거 라우터 / `docs/*.md` = 개념·계약 /
`.claude/commands/*.md` = 절차** 의 책임 분담을 따릅니다. 각 md 는 자기 역할 안에서만
정보를 보유하고, 인접 도메인은 단일 출처(SoT) 위임으로 처리합니다.

> **md 수정 시 체크 트리거**: 아래 4종 수정은 시작 전 위 분담을 깨지 않는지 한 번 확인.
> 그 외 수정(오타·문장 다듬기·경로 rename 반영·상수 갱신)은 체크 생략.
>
> 1. 새 `##` 섹션을 `docs/*.md` 또는 `.claude/commands/*.md` 에 추가 — 중복 가능성 최대. 추가 전 인접 md grep.
> 2. `.claude/commands/*.md` 에 *개념적 설명* 추가 — docs 영역 침투. "왜"는 docs, commands 는 포인터만.
> 3. `docs/*.md` 에 *step-by-step 절차* 추가 — commands 영역 침투. "어떤 명령을 친다"는 commands.
> 4. CLAUDE.md 에 새 문단 추가 — always-on 컨텍스트라 슬림 유지. 새 도메인은 트리거 한 줄 + 문서 위임.

### 현재-상태 원칙 (no-legacy) (메타 규칙: 그대로 유지 권장)

md 와 설명 주석(docstring 포함)은 **현재 구현만** 기술합니다. 로직을 수정하면 그 동작을
서술하던 md·주석을 같은 작업 안에서 **완전히 교체**하고, 옛 동작의 흔적("예전에는 X였으나",
"(구) 방식", "~로 변경됨" 류)을 남기지 마세요. 변경 이력의 단일 출처는 git 커밋 메시지입니다.

> 예외: 코드가 **지금도 실제로 지원하는** 하위호환 동작의 명세는 이력 서술이 아니라 현재 기능
> 문서이므로 유지 — 단, 해당 호환 코드가 제거되면 명세도 같은 작업에서 함께 삭제.
