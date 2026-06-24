# Storage 아키텍처

본 문서는 Storage 계층의 **계약 · 시멘틱 · 불변식**을 정리합니다.
인접 도메인은 위임으로 처리합니다:
- 시스템 기동/아키텍처 → `docs/architecture.md`
- 큐/워커 → `docs/queue.md`

> **트리거**: `internal/storage/**` 파일들을 추가/수정할 때는 시작 전 본 문서를 먼저 읽고 계약/시멘틱을 확인할 것.

---

## 1. 컴포넌트 개요

| 컴포넌트 | 파일 | 역할 |
| --- | --- | --- |
| `Provider` (인터페이스) | `internal/storage/provider.go` | 클라우드 스토리지 백엔드가 따라야 할 **계약 파일**. |
| `GoogleDriveAdapter` | `internal/storage/googledrive.go` | 구글 드라이브 구현체. Path Cache 갱신, Chunk 업로드/다운로드. |

---

## 2. Path Cache 시멘틱

일부 클라우드 스토리지는 경로(`/a/b.txt`) 기반이 아닌 Object ID 기반입니다.
잦은 폴더 트리 탐색(API 호출)을 방지하기 위해 `path_cache` 테이블을 사용합니다.
단, Cache는 보조 수단이며(Advisory), 미스 시 반드시 실제 API를 호출해 갱신해야 합니다.

---

## 3. 계약 / 불변식

1. **인터페이스 종속**: Worker Pool은 반드시 `Provider` 인터페이스만 참조해야 합니다. (구체 타입 직접 참조 금지)
2. **Chunking 위임**: 파일의 분할 업로드/다운로드(Chunk 사이즈 제어)는 Worker가 아닌 Provider 구현체의 책임입니다.
3. **Context Cancellation**: 모든 Provider 메서드는 `ctx`를 수신하며 종료 시그널(Graceful shutdown) 발생 시 I/O를 현재 진행중인 Chunk 단위에서 깔끔하게 중단/정리해야 합니다.

---

## 4. 확장 — 새 Storage Provider 추가 (additive)

1. `internal/storage/<새_프로바이더>.go` 생성.
2. `Provider` 인터페이스 구현.
3. 초기화(Init) 함수에서 설정 맵핑 연동.

> 절차(파일 레이아웃·검증 명령)는 `.claude/commands/new-storage.md` 가 자기완결적으로 다룹니다.
