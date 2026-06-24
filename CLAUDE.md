# CLAUDE.md

이 파일은 AI 에이전트가 본 저장소에서 작업할 때 참고하는 단일 하네스 가이드라인입니다. (AGENTS.md는 이 파일을 가리키는 바로가기 링크로 동작합니다.)

## 1. 런타임 및 개발 환경
- Go 1.25+. 소스 루트는 `cmd/`, `internal/` 이며 Go Module(`go.mod`) 규약.
- **개발 및 테스트 환경 (Docker 컨테이너)**: 환경 격리를 위해 `hippodrop-dev` 컨테이너 내부에서 작업을 진행합니다.
  - 빌드/구동 스크립트: `./build/docker/run-dev.sh`
  - 소스코드 마운트 위치: 컨테이너 내부 `/app` (Go 1.25.11)
  - 빌드 실행: `docker exec -w /app hippodrop-dev make build`
  - 테스트 실행: `docker exec -w /app hippodrop-dev go test ./...`
- **포트 및 데이터**: 기본 포트는 **8080**이며, 컨테이너 내외로 포트가 선점되어 있지 않아야 합니다.

## 2. 엔트리포인트 / 노드 실행
- `cmd/hippo/main.go` 가 시작점. `hippo start <project>` 형태로 데몬 기동.

## 3. 검증 및 확장 절차
- **검증**: 단일/다중 컴포넌트 변경 시 `/verify` (`.claude/commands/verify.md`) 절차를 따릅니다.
- **확장**: 새 Storage 백엔드 추가 시 `/new-storage` (`.claude/commands/new-storage.md`) 절차를 따릅니다.

## 4. 트리거 라우팅 (핵심 계약)
> 아래 파일/경로 수정 시작 전, 단일 출처 계약 문서를 먼저 읽고 작동 모델을 동기화하십시오.

| 건드리는 대상 (경로/파일) | 먼저 읽을 문서 | 비고 |
| --- | --- | --- |
| `cmd/hippo/**` | [architecture.md](file:///mnt/c/Users/admin/projects/personal/hippo-drop/docs/architecture.md) | 싱글 바이너리 제약, 종료 시퀀스 |
| `internal/storage/**` | [storage.md](file:///mnt/c/Users/admin/projects/personal/hippo-drop/docs/storage.md) | 구글 드라이브 구현체 명세 |
| `internal/queue/**`, `internal/worker/**` | [queue.md](file:///mnt/c/Users/admin/projects/personal/hippo-drop/docs/queue.md) | SQLite 큐, 워커풀 동시성 |
| `internal/api/**` | [api.md](file:///mnt/c/Users/admin/projects/personal/hippo-drop/docs/api.md) | HTTP API 및 비동기 처리 계약 |
| Git 작업 및 커밋 작성 시 | [CONTRIBUTING.md](file:///mnt/c/Users/admin/projects/personal/hippo-drop/CONTRIBUTING.md) | 커밋 포맷, 브랜치 전략 |

*주의: `internal/storage/provider.go`는 플랫폼 인터페이스 정의 파일이므로 확장이 아닌 변경 시에만 극히 신중히 수정하십시오.*

## 5. 개발 핵심 제약 사항 (Core Constraints)
- **외부 종속성 추가 금지** — 사용자 허락 없이 `go.mod/go.sum` 수정 금지.
- **단일 바이너리 제약** — Redis, RabbitMQ 등 외부 브로커 도입 금지.
- **SQLite 큐 상태 보존** — 워커 상태는 무조건 SQLite에 저장 및 동기화.
- **Graceful Shutdown** — `SIGTERM → drain → flush → exit` 시퀀스 보존.
- **Rate Limiter 활성화** — Token Bucket 토큰 버킷 속도 제한기는 테스트 중에도 바이패스 금지.

## 6. AI 에이전트 행동 규칙 (AI Constraints - 필수)
- **생각 및 답변 언어**: 생각(Reasoning)은 **English**, 사용자 답변은 **한국어**로 정형화된 정중한 어조 사용.
- **선분석 후코딩 (Strict)**: 사용자 지시 없이 소스코드를 선제 수정 금지. (1) 문제 원인 분석 -> (2) 해결 방안 제안 및 `/grill-me` -> (3) 사용자 승인 후 코드 수정 진행. (진단 답변은 코드 수정 명령이 아닙니다.)
- **Grill-me 적극 사용**: 설계가 모호하거나 SQLite 스키마, 스토리지 인터페이스, 공용 API 규약 변경 시 반드시 `/grill-me` 세션 진행.
- **원자적 커밋 (Atomic Commits)**: 변경점을 단일 커밋으로 묶지 말고 기능별 분리 커밋 수행.
- **이력 투명성**: 실수 발생 시 숨기지 말고 즉시 보고.
- **브랜치 규칙**: 모든 작업은 `develop` 브랜치에서 진행하며 `main` 직접 커밋은 금지.

## 7. md 파일 수정 모듈성 체크 (메타 규칙)
- `CLAUDE.md` = 트리거 라우터 / `docs/*.md` = 개념·계약 / `.claude/commands/*.md` = 절차 규칙 준수.
- md 및 주석은 현재 구현만 기술(no-legacy 원칙).
