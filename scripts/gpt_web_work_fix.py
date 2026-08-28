from pathlib import Path

path = Path("scripts/gpt_web_work_apply.py")
text = path.read_text()
old = r"(?:\\\\."
new = r"(?:\\."
count = text.count(old)
if count != 2:
    raise SystemExit(f"expected 2 over-escaped OpenAPI regex fragments, found {count}")
path.write_text(text.replace(old, new))
Path(__file__).unlink()
