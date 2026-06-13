# 📄 시스템 설계서: Tardis (Storage Gateway Proxy)

## 1. 개요 (Overview)

본 문서는 로컬 개발 환경/MSA 구동환경의 메인 애플리케이션(AI 파이프라인, 데이터 프로세싱 등)과 클라우드 스토리지 간의 네트워크 I/O 병목을 해결하기 위한 경량 비동기 스토리지 게이트웨이인 **Tardis**의 아키텍처 명세입니다. Nginx와 같은 단일 실행 파일(Standalone Binary) 형태로 구동되며, 외부 의존성 없이 Raw OS 데몬이나 초경량 컨테이너 환경에서 독립적으로 동작합니다.

### 1.1. 시스템 목적

- 단일 바이너리(Single Binary) 실행을 통한 배포 및 유지보수 복잡도 최소화.
- 비동기 작업 큐잉을 통한 메인 애플리케이션의 처리 지연(Latency) 원천 차단.
- 시스템 재부팅이나 프로세스 강제 종료 시에도 작업 상태를 보존하는 영속성 확보.
- 클라우드 API 호출 제한(Rate Limit)을 준수하는 트래픽 스로틀링(Throttling).

## 2. 아키텍처 및 기술 스택 (Architecture & Tech Stack)

모든 구성 요소는 단일 프로세스 내에서 가벼운 스레드 형태로 동작하며, 외부 인프라(Redis, Celery 등)에 의존하지 않습니다.

- **개발 언어:** `Go (Golang)` - 크로스 컴파일, 정적 링크(Static Linking), Goroutine을 활용한 고효율 동시성 처리.
- **내장 데이터베이스:** `SQLite` - 작업 큐(Queue) 영속화 및 디렉토리-ID 맵핑 캐시 저장소.
- **설정 관리:** `YAML` 포맷 기반 외부 설정 파일 파싱.
- **배포 형태:** OS별 네이티브 바이너리 (Linux, macOS, Windows) 및 Scratch 기반 초경량 Docker 컨테이너.

## 3. 핵심 컴포넌트 상세 설계

### 3.1. API 프록시 계층 (HTTP Server)

로컬 애플리케이션으로부터 스토리지 입출력 요청을 수신하는 진입점입니다.
- 들어온 요청(파일 업로드, 프리페치 다운로드)의 유효성을 검증하고, 내장 SQLite 큐에 작업을 기록합니다.
- 파일 I/O나 네트워크 통신을 기다리지 않고 즉시 `202 Accepted` 상태와 `task_id`를 반환합니다.

### 3.2. 상태 영속화 및 캐시 계층 (SQLite Storage)

메모리 휘발로 인한 데이터 유실을 막기 위한 내부 저장소입니다.

- **Queue Table:** 대기 중인 작업, 진행 중인 작업, 실패(재시도 대기) 작업의 메타데이터와 상태를 추적합니다.
- **Path Cache Table:** 구글 드라이브의 플랫(Flat) 구조 극복을 위해, 로컬 경로 문자열(`/Backup/Logs`)과 클라우드 객체 ID(`1BxiM...`) 간의 매핑 정보를 영구 캐싱하여 불필요한 API 검색을 생략합니다.

### 3.3. 비동기 워커 풀 (Goroutine Worker Pool)

설정 파일에 정의된 워커 수(`pool_size`)만큼 생성되어 백그라운드에서 큐의 작업을 처리합니다.

- **Rate Limiter 내장:** 토큰 버킷(Token Bucket) 알고리즘을 적용하여 초당 API 호출 횟수를 엄격하게 제어합니다.    
- **Exponential Backoff:** 네트워크 단절이나 403 에러 발생 시, 점진적으로 대기 시간을 늘리며 재시도합니다.
- **Graceful Shutdown:** `SIGTERM` 신호 수신 시, 현재 진행 중인 청크 전송을 안전하게 마무리하고 큐 상태를 디스크에 확정한 뒤 프로세스를 종료합니다.

### 3.4. 스토리지 어댑터 인터페이스 (Storage Provider)

스토리지 통신 로직을 추상화하여 확장성을 보장합니다.

- `StorageProvider`라는 공통 인터페이스(`Upload`, `Download`, `GetPathId` 등)를 정의합니다.
- 초기 구현체로 `GoogleDriveAdapter`를 장착하며, 내부적으로 OAuth2 Refresh Token을 활용한 1회용 Access Token 갱신 로직을 포함합니다.

## 4. 설정 파일 명세 (tardis.yml)

바이너리 실행 시 `-c` 플래그로 주입되며, 동작 환경의 모든 파라미터를 제어합니다.

```yaml
# /etc/tardis/tardis.yml
server:
  port: 8080
  data_dir: "/var/lib/tardis/data"  # SQLite DB 및 임시 파일 저장 경로

storage:
  provider: "google_drive"
  google_drive:
    credentials_path: "/etc/tardis/credentials.json"
    rate_limit_per_second: 10
    retry_max_attempts: 5

workers:
  pool_size: 4
  chunk_size_mb: 10  # 대용량 파일 분할 전송 단위
```

## 5. 배포 및 실행 시나리오

### 5.1. Raw OS 데몬 구동 (Linux/macOS)

의존성 패키지 설치 없이 다운로드 직후 백그라운드 서비스로 등록됩니다.

1. 바이너리 이동: `mv tardis-linux-amd64 /usr/local/bin/tardis`
2. 설정 파일 배치: `/etc/tardis/tardis.yml` 생성.
3. Systemd/Launchd 서비스 등록 후 `systemctl start tardis` 실행.

기동 과정에서의 필요한 (사용자 측면에서), 

### 5.2. 초경량 Docker 기반 구동

운영체제 레이어조차 없는 가장 가벼운 형태의 컨테이너를 구성합니다.

```Dockerfile
# Dockerfile
FROM scratch
COPY build/tardis-linux-amd64 /tardis
COPY config/tardis.yml /etc/tardis.yml
COPY config/credentials.json /etc/credentials.json

EXPOSE 8080
ENTRYPOINT ["tardis", "-c", "/path/of/directory/tardis.yml"]
```

- 로컬 큐 데이터 보존을 위해 실행 시 SQLite 저장 경로만 도커 볼륨(`-v`)으로 마운트합니다.

이상으로 Tardis 프로젝트의 시스템 설계 명세를 마칩니다. 본 설계는 로컬 컴퓨팅 자원 소모를 최소화하면서도 완벽한 비동기 데이터 파이프라인을 구축할 수 있는 독립 실행형 인프라의 기준점이 됩니다.