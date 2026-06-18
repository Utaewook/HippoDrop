# /verify — 단일 컴포넌트 변경 검증 (Tardis 네이티브)

변경한 컴포넌트를 **실제 실행 환경에서** 기동해 검증합니다: 기동 → 로그 헬스 확인 →
(선택) 동작 ping → 정리. 호스트 직접 실행이나 단위 테스트 단독 종료 금지.

근거: `docs/_AUTHORING.md` §6 (검증 규율).

## 입력 인자

- `--keep` — 검증 후 종료하지 않음 (로그 추가 조사용).

## 절차

### §1 사전 점검
- 저장소 루트 확인. `git status` / `git diff --stat` 로 변경 범위 요약.
- 동명 프로세스 잔존 확인: `lsof -i :8080` (기본 포트 점유 확인).
- `go build` 가능 여부 확인: `make build`

### §2 기동
```bash
./build/tardis start -c tardis.example.yml &  # background 실행
echo $! > tardis_test.pid
```

### §3 로그 헬스 확인 (필수)
비동기 기동이면 "기동 성공" 보고 전에 **헬스 라인을 직접** 봐야 합니다.
기대:
- `Server running on port 8080`
- `Database initialized`
- Traceback / ERROR / CRITICAL 없음

### §4 동작 ping (선택)
```bash
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/health   # 기대 200
```

### §5 정리
- `--keep` 미지정 시 프로세스 정지.
- `kill -TERM $(cat tardis_test.pid)`
- 실패 시 **즉시 파괴 금지** — 로그 확인 후 정리.

### §6 보고
빌드 결과, 헬스 결과, ping 결과, 정리 여부.

## 검증 / 헬스 로직 (실패 패턴)

| 증상 | 원인 | 조치 |
| --- | --- | --- |
| `Address already in use` | 포트 점유 | §1 회귀 (잔존 프로세스 정리) |
| `database locked` | 다중 인스턴스가 같은 data_dir 점유 | `tardis.example.yml`의 data_dir 확인 |
| 응답코드 4xx/5xx | API 오류 | `docs/api.md` 계약 확인 |
