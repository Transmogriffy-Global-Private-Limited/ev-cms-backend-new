"""Text-only pre-CoW 000078/000079 migration consistency checks, not PG proof.

Safe in the real repository alongside older and future migrations. Uses explicit
checks so python -O cannot silently skip validation.
"""
from collections import Counter
from pathlib import Path
import re
import sys


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def verify(root: Path) -> None:
    migrations = root / "db" / "migrations"
    target_bases = {
        "000078_pre_cow_notice_and_gst_integrity",
        "000079_pre_cow_charging_tenant_integrity",
    }
    target_names = {f"{base}.{direction}.sql" for base in target_bases for direction in ("up", "down")}
    all_files = sorted(migrations.glob("*.sql"))
    require(bool(all_files), "no SQL migrations found")
    known = re.compile(r"^([0-9]{6})_.+\.(up|down)\.sql$")
    groups = {}
    for path in all_files:
        match = known.fullmatch(path.name)
        require(match is not None, f"unrecognized migration filename: {path.name}")
        version, direction = match.groups()
        groups.setdefault(version, {}).setdefault(direction, []).append(path.name)
    for version, paired in groups.items():
        require(len(paired.get("up", [])) == 1 and len(paired.get("down", [])) == 1,
                f"incomplete or duplicate migration version {version}: {paired}")
    found = {p.name for p in all_files if p.name.startswith(("000078_", "000079_"))}
    require(found == target_names, f"expected exact 078/079 migration files; missing={sorted(target_names-found)} unexpected={sorted(found-target_names)}")

    content = {name: (migrations/name).read_text(encoding="utf-8") for name in target_names}
    for name, sql in content.items():
        require(len(sql.strip()) > 200, f"migration too short: {name}")
        no_comments = re.sub(r"(?m)^\s*--[^\n]*", "", sql)
        require(not re.search(r"\b(?:DROP\s+TABLE|TRUNCATE)\b", no_comments, re.I), f"destructive SQL in {name}")
        require(not re.search(r"(?im)^\s*(?:DELETE\s+FROM|UPDATE\s+\w+\s+SET|INSERT\s+INTO)\b", no_comments),
                f"row mutation SQL in {name}")
        require(sql.count("$$") % 2 == 0, f"unbalanced SQL dollar delimiters: {name}")
    f78 = content["000078_pre_cow_notice_and_gst_integrity.up.sql"]
    f79 = content["000079_pre_cow_charging_tenant_integrity.up.sql"]
    for name, sql in (("078", f78), ("079", f79)):
        for phrase in ("ALTER TABLE", "NOT VALID", "VALIDATE CONSTRAINT", "ON DELETE RESTRICT"):
            require(phrase in sql, f"migration {name} missing {phrase}")
    require("uq_platform_notifications_platform_recipient" in f78, "078 lacks platform-recipient uniqueness")
    require("ON DELETE NO ACTION" in f78 and "DEFERRABLE INITIALLY DEFERRED" in f78, "078 lacks deferred target integrity")
    require("ON DELETE SET NULL" not in re.sub(r"--[^\n]*", "", f78), "078 retains invalid SET NULL")
    for phrase in ("wallets(cpo_id, customer_id, id)",
                   "FOREIGN KEY (cpo_id, customer_id, wallet_id)",
                   "FOREIGN KEY (cpo_id, customer_id, session_id)"):
        require(phrase in f79, f"079 missing {phrase}")
    for version, sql, expected in (("078", f78, 2), ("079", f79, 16)):
        names = re.findall(r"ADD CONSTRAINT\s+([a-zA-Z_][\w]*)\s+FOREIGN KEY", sql, re.I)
        validated = re.findall(r"VALIDATE CONSTRAINT\s+([a-zA-Z_][\w]*)", sql, re.I)
        require(len(names) == expected, f"migration {version} expected {expected} FKs; found {len(names)}")
        require(Counter(names) == Counter(validated), f"migration {version} FK names/validations mismatch")
        require(all(len(name) <= 63 for name in names), f"migration {version} constraint name too long")
    test = (root/"db"/"pre_cow_integrity_migration_integration_test.go").read_text(encoding="utf-8")
    for phrase in ("TEST_DATABASE_URL", "CREATE SCHEMA", "ROLLBACK",
                   "TestPreCoWNoticeAndGSTIntegrityWithPostgreSQL", "TestPreCoWChargingTenantIntegrityWithPostgreSQL"):
        require(phrase in test, f"missing isolated PG integration test marker: {phrase}")
    for name in ("pre_cow_integrity_readonly_preflight.sql", "pre_cow_integrity_readonly_postflight.sql"):
        sql = (root/"scripts"/name).read_text(encoding="utf-8")
        require("BEGIN READ ONLY;" in sql and "COMMIT;" in sql, f"not read-only transaction: {name}")
        require(not re.search(r"(?im)^\s*(?:ALTER|UPDATE|DELETE|INSERT|TRUNCATE|DROP|CREATE)\s+", sql),
                f"mutating statement in read-only verification script: {name}")
    print("Static package checks PASS: migrations 078/079, 18 FKs, two isolated PG test cases, read-only SQL scripts.")
    print("Scope: source checks only; no PostgreSQL execution or release-readiness assertion.")


if __name__ == "__main__":
    try:
        require(len(sys.argv) == 1, "this script accepts no arguments")
        verify(Path(__file__).resolve().parent.parent)
    except (OSError, RuntimeError, ValueError) as error:
        print(f"Static package checks FAIL: {error}", file=sys.stderr)
        sys.exit(1)
