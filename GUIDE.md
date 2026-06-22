# GUIDE — 에이전트 하네스 및 작업 지침 관리 가이드

본 문서는 `tardis` 저장소에 이미 구축된 에이전트 하네스(CLAUDE.md, `docs/*.md`, `.claude/commands/*.md`)를 올바르게 유지보수하고 향후 개발 및 확장 시 활용하는 규칙을 가이드합니다. 

이 문서의 작성 규칙 및 아키텍처적 근거는 전부 [docs/_AUTHORING.md](file:///mnt/c/Users/admin/projects/personal/tardis/docs/_AUTHORING.md) 가 단일 출처입니다.

---

## 1. 하네스의 기본 구조와 핵심 가치

Tardis 저장소는 인공지능 에이전트(Antigravity 등)가 소스코드를 분석하고 안전하게 수정할 수 있도록 정밀하게 설계된 **하네스(Harness)** 환경을 유지합니다.

* **트리거 라우터 (CLAUDE.md)**: 
  * "어느 경로/파일을 수정할 때 어떤 가이드 문서를 먼저 읽어야 하는가"를 매핑하는 항상 열려 있는 컨텍스트 인터페이스입니다.
* **개념 및 계약 문서 (docs/*.md)**:
  * 각 도메인(스토리지, API, 큐 등)의 구체적인 구현 제약, 불변식, 아키텍처 규칙을 설명합니다.
* **절차 및 명령 (commands/*.md)**:
  * 컴포넌트 추가나 작업 검증과 같이 반복 실행해야 할 행위들의 Step-by-step 절차를 자기완결적으로 기록합니다.

---

## 2. 하네스 유지보수 및 확장 절차

### 1단계: 신규 도메인/컴포넌트 추가 시 (예: 새 API 모듈)
1. [docs/_TEMPLATE.md](file:///mnt/c/Users/admin/projects/personal/tardis/docs/_TEMPLATE.md)를 복제하여 `docs/<도메인>.md` 개념 문서를 만듭니다.
2. 컴포넌트 구조, 왜 그렇게 설계되었는지(Why), 어겨선 안 될 제약을 기술합니다.
3. [CLAUDE.md](file:///mnt/c/Users/admin/projects/personal/tardis/CLAUDE.md)의 트리거 라우팅 표에 새 파일 경로 매핑 규칙을 한 줄 추가합니다.

### 2단계: 신규 스토리지 백엔드 추가 시 (예: S3, Azure Blob 등)
1. [.claude/commands/new-storage.md](file:///mnt/c/Users/admin/projects/personal/tardis/.claude/commands/new-storage.md) 커맨드 파일의 절차를 그대로 이행합니다.
2. `internal/storage/<backend_name>.go` 파일에 `storage.Provider` 인터페이스를 구현하고, [config.go](file:///mnt/c/Users/admin/projects/personal/tardis/internal/config/config.go)와 [main.go](file:///mnt/c/Users/admin/projects/personal/tardis/cmd/tardis/main.go)를 순서대로 수정 및 연동합니다.

### 3단계: 로컬 검증 실행 시
1. 소스코드나 구성을 변경한 뒤에는 반드시 [.claude/commands/verify.md](file:///mnt/c/Users/admin/projects/personal/tardis/.claude/commands/verify.md) 가이드에 따라 실제 실행 환경(네이티브 Go 런타임)에서 데몬을 기동하여 검증합니다.
2. 훅으로 등록된 `.claude/hooks/syntax_check.py`가 Go 파일(`.go`)에 대해 `gofmt`를 사용해 구문 검사를 수행하므로, 편집 결과물에 문법 오류가 없는지 자동으로 점검됩니다.

---

## 3. 작업 시 주의 및 금지 사항

* **SoT(단일 출처) 위배 금지**: 동일한 규칙이나 시맨틱을 여러 마크다운 문서에 중복해서 적지 마세요.
* **계약 파일 마음대로 수정 금지**: `internal/storage/provider.go`와 같이 아키텍처 계약을 정의한 인터페이스 파일은 플랫폼 설계 수준의 변경이 아닌 이상 함부로 수정해서는 안 되며, 수정을 시도할 경우 `contract_guard` 훅이 경고 및 차단 처리를 수행합니다.
* **현재-상태 원칙 (No-legacy)**: 모든 마크다운과 주석은 과거 이력을 배제하고 **오직 현재 코드의 동작 상태**만을 설명해야 합니다. 이력 관리는 Git Commit이 수행합니다.
