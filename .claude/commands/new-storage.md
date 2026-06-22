# /new-storage — 새 Storage 백엔드 추가

`internal/storage/` 에 새 Storage 백엔드(Provider)를 추가하고 디스패처에 연결합니다.
계약·시멘틱의 근거는 [storage.md](file:///mnt/c/Users/admin/projects/personal/tardis/docs/storage.md) 가 단일 출처이며, 본 커맨드는 절차만 다룹니다.

## 입력 인자

- `backend_name` — snake_case (파일 이름 및 프로바이더 타입명 기준, 예: `s3`, `local_disk`).
- `description` — 선택.

## 절차

> step 번호(§N)는 다른 커맨드/문서가 인용하는 **안정 앵커**입니다. 재배치 금지, 새 step 은 append.

### §1 참고 구현 읽기 + 충돌 점검
- 기존 백엔드 구현 파일(예: [googledrive.go](file:///mnt/c/Users/admin/projects/personal/tardis/internal/storage/googledrive.go))을 참고용으로 읽습니다.
- `internal/storage/<backend_name>.go` 가 이미 존재하는지 확인하여 덮어쓰기를 방지합니다.

### §2 백엔드 어댑터 파일 생성
- `internal/storage/<backend_name>.go` 파일을 생성합니다.
- `storage.Provider` 인터페이스의 세 메서드를 구현하는 struct를 선언합니다:
  - `Upload(ctx context.Context, localPath, remotePath string) error`
  - `Download(ctx context.Context, remotePath, localPath string) error`
  - `GetPathID(ctx context.Context, path string, createIfMissing bool) (string, error)`
- 설정 로드 시 사용할 `New<Name>Adapter` 팩토리 함수를 작성합니다.

### §3 설정 구조체 및 디스패처 연결
- [config.go](file:///mnt/c/Users/admin/projects/personal/tardis/internal/config/config.go) 의 `StorageConfig` 구조체에 해당 프로바이더 전용 설정 구조체(예: `<Name>Config`)를 추가하고 YAML 태그를 매핑합니다.
- [main.go](file:///mnt/c/Users/admin/projects/personal/tardis/cmd/tardis/main.go) 의 storage provider 초기화 단락(444~452행 부근)에 `cfg.Storage.Provider == "<backend_name>"` 분기를 추가하고, 생성한 팩토리 함수를 호출하여 `provider` 변수에 할당합니다.
- **기존 분기 로직(예: google_drive)은 수정 금지** — 오직 신규 분기만 추가합니다.

### §4 정적 검증
- 작성한 백엔드가 `storage.Provider` 인터페이스를 올바르게 구현했는지 빌드를 통해 점검합니다.
  ```bash
  make build
  ```

### §5 실제 환경 검증
- 개발용 설정 파일(`tardis.example.yml` 등)에 새 프로바이더 설정을 기입하고 `/verify` 슬래시 커맨드를 호출하여 기동 성공 여부 및 로그 헬스를 점검합니다.

### §6 보고
- 추가된 어댑터 파일 경로, [config.go](file:///mnt/c/Users/admin/projects/personal/tardis/internal/config/config.go) 및 [main.go](file:///mnt/c/Users/admin/projects/personal/tardis/cmd/tardis/main.go) 수정 내역, 검증 결과를 요약하여 보고합니다.

## 핵심 불변식 / 계약

- 계약 파일([provider.go](file:///mnt/c/Users/admin/projects/personal/tardis/internal/storage/provider.go))의 **인터페이스는 수정 금지** — 플랫폼 변경 시에만 제한적으로 허용합니다.
- 비밀번호, API 키 등 민감정보는 설정 파일에 평문 삽입을 금지하며 환경변수로 주입받도록 가이드합니다.

## 검증 / 헬스 로직

| 증상 | 원인 | 조치 |
| --- | --- | --- |
| 빌드 오류 (Interface implementation) | 인터페이스 시그니처 불일치 | `Upload`, `Download`, `GetPathID` 메서드 타입 및 파라미터가 `Provider`와 일치하는지 확인 |
| Unsupported storage provider | 디스패처 분기 누락 또는 오타 | `cmd/tardis/main.go` 초기화 분기 문자열 검토 |
