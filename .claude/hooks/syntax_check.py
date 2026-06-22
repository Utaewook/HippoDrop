#!/usr/bin/env python3
"""PostToolUse hook: Edit/Write 후 파일이 구문 오류로 남았으면 모델에 즉시 피드백.

언어 무관 — 확장자별 검사기를 CHECKERS 에 등록. 등록 안 된 확장자는 무시(통과).
Exit 2 → stderr 가 모델에 전달되어 즉시 수정하게 함. Exit 0 → 통과.

새 언어 추가하기:
    1) 검사 함수를 하나 정의 (str source -> (ok: bool, msg: str)).
    2) CHECKERS 에 확장자(소문자, 점 포함)를 키로 등록.
외부 린터(eslint, gofmt 등)를 쓰려면 subprocess 로 호출하는 함수를 만들어 등록하면 됨.
"""
import ast
import json
import sys
from pathlib import Path

try:
    sys.stderr.reconfigure(encoding="utf-8")
except Exception:
    pass


def _check_python(source: str):
    try:
        ast.parse(source)
        return True, ""
    except SyntaxError as exc:
        line = exc.lineno
        text = (exc.text or "").rstrip()
        return False, f"SyntaxError at line {line}: {exc.msg}\n  {text}"


def _check_json(source: str):
    try:
        json.loads(source)
        return True, ""
    except json.JSONDecodeError as exc:
        return False, f"JSONDecodeError at line {exc.lineno} col {exc.colno}: {exc.msg}"


def _check_go(source: str):
    import subprocess
    try:
        res = subprocess.run(
            ["gofmt", "-e"],
            input=source,
            text=True,
            capture_output=True,
            check=False
        )
        if res.returncode != 0:
            return False, res.stderr.strip()
        return True, ""
    except FileNotFoundError:
        # gofmt가 설치되어 있지 않은 환경에서는 경고 없이 통과
        return True, ""
    except Exception as exc:
        return False, f"Unexpected error during gofmt check: {exc}"


# 확장자(소문자, 점 포함) -> 검사 함수
CHECKERS = {
    ".py": _check_python,
    ".json": _check_json,
    ".go": _check_go,
    # ".ts": _check_typescript,   # 예: subprocess 로 tsc --noEmit 호출
}


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except Exception:
        return 0

    tool_input = payload.get("tool_input", {}) or {}
    raw_path = tool_input.get("file_path") or ""
    if not raw_path:
        return 0

    path = Path(raw_path)
    checker = CHECKERS.get(path.suffix.lower())
    if checker is None or not path.is_file():
        return 0

    try:
        source = path.read_text(encoding="utf-8")
    except Exception as exc:
        print(f"[syntax-check] could not read {path}: {exc}", file=sys.stderr)
        return 0

    ok, msg = checker(source)
    if not ok:
        print(f"[syntax-check] {path}: {msg}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
