<!-- ============================================================
     [예시 커맨드] 패턴 시범용으로 채워진 스캐폴드 커맨드입니다.
     짝 개념 문서: docs/example-storage.md
     패턴이 익으면 지우거나 자기 확장 작업용 커맨드로 교체하세요.
     ============================================================ -->

# /new-storage — 새 Storage 백엔드 추가 (예시)

`src/store/storage_imp/` 에 새 Storage 백엔드를 만들고 `StorageApi` 디스패처에 연결합니다.
계약·시멘틱의 근거는 `docs/example-storage.md` 가 단일 출처이며, 본 커맨드는 절차만 다룹니다.

## 입력 인자

- `backend_name` — snake_case (파일/클래스 이름의 기준).
- `family` — `db` | `disk` | `new` (디스패처 분기 종류).
- `description` — 선택.

## 절차

### §1 참고 구현 읽기 + 충돌 점검
- family 에 맞는 기존 backend 파일을 참고로 읽음.
- `src/store/storage_imp/<backend_name>.py` 가 이미 있으면 중단(덮어쓰기 확인 전까지).

### §2 백엔드 파일 생성
- `Storage` ABC 의 모든 추상 메서드 구현 스켈레톤 작성 (`get_max_value` 포함).
- `__init__` 은 추출된 세부 dict 를 받음 (전체 config 아님).
- docstring 에 chunk_size 의미 / PK 정책 / idempotency 정책 명시.

### §3 디스패처 연결
- `StorageType`(또는 `DbType`) enum 에 상수 추가.
- `StorageApi.create_*` 에 분기 한 줄 + import 추가.
- **기존 분기 로직은 변경 금지** — 신규 분기만.

### §4 정적 검증
- 클래스명 ↔ 파일명 일치 확인.
- `apply_schema` 를 **연속 2회 이상** 호출해 idempotency 확인.

### §5 실제 환경 검증
- 새 backend 를 쓰는 최소 element/설정으로 `/verify` 실행.

### §6 보고
- 만든 파일 경로, 추가한 enum/분기, 검증 결과, 다음 단계.

## 핵심 불변식 / 계약

- 계약 파일(`storage.py`, `storage_api.py`)의 **인터페이스는 수정 금지** — enum 추가/새 분기만.
- `apply_schema` idempotency, thread-safety, binary 이중 인코딩 금지 (근거: `docs/example-storage.md` §3).
- 비밀정보 평문 삽입 금지 — 환경변수 참조 플레이스홀더로.

## 검증 / 헬스 로직

| 증상 | 원인 | 조치 |
| --- | --- | --- |
| 구현했는데 dispatch 안 됨 | §3 디스패처 분기 누락 | `storage_api.py` 분기 추가 |
| reload 마다 schema 오류 | non-idempotent `apply_schema` | §4 재점검 |
