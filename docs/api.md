# API 아키텍처

본 문서는 HTTP 통신 계층의 **계약 · 시멘틱 · 불변식**을 정리합니다.

> **트리거**: `internal/api/**` 파일들을 추가/수정할 때는 시작 전 본 문서를 먼저 읽고 계약/시멘틱을 확인할 것.

---

## 1. 컴포넌트 개요

| 컴포넌트 | 파일 | 역할 |
| --- | --- | --- |
| `Handler` | `internal/api/handler.go` | 업로드, 다운로드, 태스크 상태 조회 엔드포인트. |

---

## 2. API 흐름 시멘틱 (Fire-and-forget)

외부에서 HTTP POST 로 `/upload` 또는 `/download` 요청이 오면:
1. 인풋 유효성 검사.
2. SQLite 큐에 Task를 Insert.
3. **즉시** `202 Accepted` 와 함께 `{task_id}` 반환.

절대로 클라우드 스토리지 API I/O 응답을 기다린 후 HTTP 요청을 종료해선 안 됩니다 (Zero latency for caller 원칙).

---

## 3. 계약 / 불변식

1. **지연 없음(Non-blocking)**: HTTP 핸들러 내부에서 Provider를 직접 호출하는 것은 절대 금지됩니다. 
2. **동기화 원천 차단**: Task 생성 후 처리 추적은 클라이언트가 `/tasks/{task_id}` 엔드포인트를 Polling 하는 방식으로만 제공됩니다.
