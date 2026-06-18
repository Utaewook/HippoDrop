# Queue 및 Worker 아키텍처

본 문서는 비동기 Task 처리 계층의 **계약 · 시멘틱 · 불변식**을 정리합니다.
인접 도메인은 위임으로 처리합니다:
- 시스템 전반/기동 → `docs/architecture.md`
- Storage Provider → `docs/storage.md`

> **트리거**: `internal/queue/**`, `internal/worker/**` 파일들을 추가/수정할 때는 시작 전 본 문서를 먼저 읽고 계약/시멘틱을 확인할 것.

---

## 1. 컴포넌트 개요

| 컴포넌트 | 파일 | 역할 |
| --- | --- | --- |
| `Task Queue` | `internal/queue/db.go` | SQLite `tasks` 테이블을 감싸는 레파지토리. |
| `Scheduler` | `internal/worker/scheduler.go` | 주기적으로 DB 폴링하여 `pending` 상태 Task 분배 |
| `Worker Pool` | `internal/worker/pool.go` | 고정된 워커 Goroutine 모음 및 Rate Limiter 통제. |

---

## 2. 분배/실행 흐름 (시멘틱)

1. Scheduler가 `status = 'pending'` 인 Task를 읽어 Go Channel로 푸시.
2. Worker가 Channel에서 Task 수신 후, SQLite의 `status`를 `running`으로 즉시 업데이트.
3. Token Bucket 에서 Rate Limiter 토큰 획득.
4. Storage Provider I/O 실행.
5. 성공 시 `done`, 에러 시 재시도(`retry_count` 증가 + `pending`) 또는 횟수 초과 시 `failed` 기록.

---

## 3. 계약 / 불변식

1. **메모리 큐 금지**: 모든 Task는 반드시 SQLite 큐 테이블에 영속되어야 합니다. 채널은 단지 분배용(버퍼용)일 뿐 큐가 아닙니다.
2. **복구 보장**: 시스템 시작 시, 이전에 비정상 종료되어 `running` 상태에 머물러 있는 Task들은 반드시 `pending`으로 리셋해야 영구 유실을 막을 수 있습니다.
3. **엄격한 속도 제한**: Token Bucket 기반 Rate Limiter는 Worker에서 I/O 실행 직전 언제나 강제되어야 하며, 테스트 모드라고 해서 무력화될 수 없습니다.
