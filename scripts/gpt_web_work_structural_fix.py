from pathlib import Path

apply_path = Path("scripts/gpt_web_work_apply.py")
text = apply_path.read_text()
start_marker = 'replace_first_after(\n    openapi,\n    "\\n    ChargingSessionView:\\n",'
start = text.find(start_marker)
if start < 0:
    raise SystemExit("fragile ChargingSessionView OpenAPI patch block not found")
end_marker = "\n# These transport files must not survive the implementation commit."
end = text.find(end_marker, start)
if end < 0:
    raise SystemExit("transport cleanup marker not found after ChargingSessionView patch")
apply_path.write_text(text[:start] + text[end:])
Path(__file__).unlink()
