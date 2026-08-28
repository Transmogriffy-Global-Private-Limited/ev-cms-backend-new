from pathlib import Path

path = Path("docs/contracts/openapi/openapi.yaml")
text = path.read_text()
anchor = "\n    ChargingSessionView:\n"
start = text.find(anchor)
if start < 0:
    raise SystemExit("ChargingSessionView schema not found")
next_schema = text.find("\n    ", start + len(anchor))
if next_schema < 0:
    next_schema = len(text)
section = text[start:next_schema]
if "projected_amount:" in section:
    raise SystemExit("ChargingSessionView already has projected_amount unexpectedly")
marker = "        total_amount:"
marker_pos = section.find(marker)
if marker_pos < 0:
    raise SystemExit("ChargingSessionView total_amount property not found")
line_end = section.find("\n", marker_pos)
if line_end < 0:
    raise SystemExit("ChargingSessionView total_amount line is unterminated")
insertion = '\n        projected_amount: {type: string, pattern: "^[0-9]+(?:\\\\.[0-9]{1,2})?$", description: "Present before final settlement is persisted; current accrued charge from immutable tariff/tax snapshots, not a settled amount."}'
section = section[:line_end] + insertion + section[line_end:]
path.write_text(text[:start] + section + text[next_schema:])
Path(__file__).unlink()
