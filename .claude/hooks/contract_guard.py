#!/usr/bin/env python3
"""PreToolUse hook: Edit/Write 대상이 '계약 파일'이면 경고(또는 차단).

계약 파일 = 플랫폼 인터페이스를 정의해, 확장이 아니라 플랫폼 변경일 때만 건드려야 하는 파일.
glob 목록은 같은 디렉토리 옆의 contract_files.txt 에서 읽음 (한 줄에 하나, # 주석/빈 줄 무시).
경로는 저장소 루트(=현재 작업 디렉토리) 기준 상대 glob 로 매칭.

MODE:
    "warn"  → 경고만 하고 통과 (exit 0). 기본값.
    "block" → 편집을 막고 모델에 사유 전달 (exit 2). 신중함을 강제하고 싶을 때.

근거/철학: docs/_AUTHORING.md §3 (계약 파일), §4 (additive 확장).
"""
import json
import sys
from pathlib import Path
from fnmatch import fnmatch

MODE = "warn"  # "warn" | "block"

try:
    sys.stderr.reconfigure(encoding="utf-8")
except Exception:
    pass


def _load_patterns():
    cfg = Path(__file__).resolve().parent.parent / "contract_files.txt"
    if not cfg.is_file():
        return []
    pats = []
    for line in cfg.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if line and not line.startswith("#"):
            pats.append(line)
    return pats


def _rel(raw_path: str) -> str:
    p = Path(raw_path)
    try:
        return str(p.resolve().relative_to(Path.cwd().resolve()))
    except Exception:
        return str(p)


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except Exception:
        return 0

    tool_input = payload.get("tool_input", {}) or {}
    raw_path = tool_input.get("file_path") or ""
    if not raw_path:
        return 0

    patterns = _load_patterns()
    if not patterns:
        return 0

    rel = _rel(raw_path)
    hit = next((pat for pat in patterns if fnmatch(rel, pat) or fnmatch(raw_path, pat)), None)
    if hit is None:
        return 0

    msg = (
        f"[contract-guard] '{rel}' 는 계약 파일입니다 (matched: {hit}).\n"
        f"  이것이 단순 확장이 아니라 의도된 '플랫폼 변경' 인지 확인하세요.\n"
        f"  확장이라면 보통 새 파일 추가 + 디스패처 한 줄로 충분합니다 (docs/_AUTHORING.md §4).\n"
        f"  계약을 바꾼다면 의존하는 docs/*.md 의 계약 서술도 같은 작업에서 갱신하세요 (§8 no-legacy)."
    )
    print(msg, file=sys.stderr)
    return 2 if MODE == "block" else 0


if __name__ == "__main__":
    sys.exit(main())
