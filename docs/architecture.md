# Architecture

본 문서는 시스템의 **기동 시퀀스 · 핵심 추상(원자 단위) · 재조정/생애주기 · 동시성 프리미티브**를
정의합니다. 도메인별 계약은 다른 문서로 위임합니다.

- Storage Provider 계약 → `docs/storage.md`
- 큐 및 워커풀 계약 → `docs/queue.md`
- HTTP API 계약 → `docs/api.md`

> **트리거**: 기동 시퀀스(조립 순서), 재조정 흐름, 생애주기 진입점, 동시성 프리미티브를
> 건드리는 작업(`cmd/tardis/**`)은 시작 전 본 문서를 먼저 읽을 것.

---

## 1. 기동 시퀀스

1. `config.yml` 파싱 및 환경 변수 주입. (단일 데몬 설정 로드)
2. `SQLite` 데이터베이스 초기화 및 Migration 실행 (경로는 config의 `data_dir` 의존).
3. `Storage Provider` 초기화 (구글 드라이브 등).
4. `Scheduler` 및 `Worker Pool` Goroutine 시작. 이전 실행의 `running` Task들을 `pending`으로 리셋.
5. `HTTP API` 서버 바인딩 및 요청 수신 시작.
종료(예외) 시 HTTP 수신 거부 -> 진행중인 Worker Chunk 완료 대기 -> SQLite 닫기 -> 프로세스 종료.

## 2. 핵심 추상 (원자 단위)

- **Task**: 파일 업로드/다운로드의 단일 논리 작업.
- 모든 Task는 인메모리가 아닌 **SQLite (`tasks` 테이블)** 에 기록되어야 유효합니다.
- HTTP 응답은 Task 생성 즉시 발생하며(Fire-and-forget), 실제 클라우드 I/O는 백그라운드 Worker가 처리합니다.

## 3. 생애주기 / 데이터 흐름 진입점

| 진입점 | 역할 |
| --- | --- |
| `HTTP API` | 외부 애플리케이션의 업로드/다운로드 큐잉 요청 수신 |
| `Scheduler` | 1초마다 SQLite를 폴링하여 `pending` Task를 색인 및 Worker에 분배 |
| `Worker Pool` | 채널로부터 Task 수신 -> Storage Provider 호출 -> 결과에 따른 SQLite 상태 업데이트 |

## 4. 매니저 / 컴포넌트 분담

1. **Native 바이너리**: 복잡한 의존성 없이 바이너리 하나로 동작.
2. **다중 데몬(Multi-Project) 아키텍처**:
   - 단일 데몬이 여러 프로젝트를 처리하는 대신, 프로젝트별로 완전히 격리된 별도의 데몬 프로세스를 구동합니다 (`tardis start <project>`).
   - 설정 파일, 포트, SQLite DB(Queue/Cache), PID 모두 `~/.tardis/projects/<project>/` 하위에 완벽히 격리됩니다.
   - `-c` 플래그나 `/etc/tardis/` 시스템 전역 설정 폴백은 배제하여 유저 권한 내에서 사이드 이펙트를 원천 차단합니다.
3. **토큰 버킷 기반 속도 제한**: 클라우드 API 호출량(Rate Limit)을 엄격히 통제. 테스트 시에도 비활성화 불가.

## 5. 동시성 프리미티브

- **Task Channel**: Scheduler -> Worker 데이터 전달. SQLite 락 경합을 최소화하기 위한 목적.
- **Worker Pool**: 고정된 수의 Goroutine으로 동작하며 런타임 동적 스케일링은 지원하지 않습니다.
- **Token Bucket**: Provider API 호출 전 엄격한 Rate Limiter 적용. 테스트 시에도 비활성화 불가.
