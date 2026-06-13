# Implementation Plan (Tardis)

이 문서는 `03_architecture.md`와 `시스템 설계서`를 기반으로 구체화한 Tardis 시스템 구현 계획입니다.

## 1. 구현 단계 (Phases)

### Phase 1: 기반 시스템 및 데이터베이스 설정
*   **작업 내용:**
    *   `internal/config`: `tardis.yml` 파싱 로직 구현 (Viper 또는 단순 YAML 파서 사용).
    *   `internal/queue`: SQLite 연결 및 초기화.
    *   `tasks` 테이블 및 `path_cache` 테이블 스키마 생성 및 마이그레이션 적용.
    *   SQLite 동시성 처리를 위한 WAL 모드 활성화 및 커넥션 풀 설정.

### Phase 2: 스토리지 추상화 및 어댑터 구현
*   **작업 내용:**
    *   `internal/storage`: `StorageProvider` 인터페이스 정의 (`Upload`, `Download`, `GetPathID`).
    *   `GoogleDriveAdapter` 구현: OAuth2 토큰 갱신, Chunk 단위 업로드 로직 작성.
    *   `path_cache` DB를 연동하여 구글 드라이브 ID 조회 최적화.

### Phase 3: 비동기 워커 풀 및 스케줄러 (가장 핵심)
*   **작업 내용:**
    *   `internal/worker`: Worker 구조체 및 풀 관리 로직 구현.
    *   토큰 버킷(Token Bucket) 기반 API Rate Limiter 구현.
    *   네트워크 오류 시 Exponential Backoff 재시도 로직 구현.
    *   스케줄러 구현: DB에서 `status='pending'`인 작업을 읽어와 Worker에 할당. (할당 방식은 설계 확정 후 구현)

### Phase 4: API 프록시 계층 구현
*   **작업 내용:**
    *   `internal/api`: HTTP 라우터 및 핸들러 구현.
    *   `POST /upload`, `POST /download`: 요청 검증 후 DB에 Task 저장, 즉시 `202 Accepted` 반환.
    *   `GET /tasks/{task_id}`: 작업 상태 폴링용 엔드포인트 구현.

### Phase 5: 애플리케이션 조립 및 Graceful Shutdown
*   **작업 내용:**
    *   `cmd/tardis/main.go`: 설정 로드, DB 연결, Worker Pool 시작, HTTP 서버 구동 등 전체 의존성 주입.
    *   `SIGTERM` 수신 시: API 요청 차단 -> 스케줄러 중지 -> 실행 중인 워커 작업 완료 대기 -> DB 정리 -> 프로세스 종료.

---

## 2. 확정된 설계 요소 (Grill-Me 세션 결과)

`/grill-me` 세션을 통해 아래 항목들이 확정되었습니다. 이 기준에 따라 코드를 구현합니다.

1.  **스케줄러 할당 방식:** Scheduler가 주기적으로 SQLite를 폴링하고, Go Channel을 통해 Worker들에게 Task를 Push합니다. (DB 부하 경감, Go 관용적 패턴)
2.  **재시작 복구 로직:** 서버 재시작 시, 기존 `running` 상태였던 작업들을 모두 `pending`으로 자동 롤백시켜 누락 없이 재시도합니다.
3.  **상태 조회 API 스키마:** `GET /tasks/{task_id}`는 `task_id`, `status`, `error_msg` 외에도 `type`, `local_path`, `remote_path`, `retry_count`, `created_at` 등 상세 메타데이터를 모두 반환합니다.
4.  **다운로드 API 스펙:** `POST /download` 엔드포인트를 사용하며, 바디는 `{"remote_path": "...", "local_path": "..."}` 입니다. 응답은 `202 Accepted`와 `task_id`를 반환합니다.
5.  **다운로드 방식:** 메모리 사용량 제어 및 실패 시 부분 재시도를 위해 HTTP Range 헤더를 이용한 Chunk 단위 다운로드(Resumable Download)를 구현합니다.
