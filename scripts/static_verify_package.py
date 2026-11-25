"""Non-database consistency checks. NOT an SQL parser or PG execution proof."""
from pathlib import Path
import re
from collections import Counter
root=Path(__file__).resolve().parent.parent
m=root/'db'/'migrations'
files=sorted(m.glob('*.sql'))
assert len(files)==4, files
assert sorted(p.name[:6] for p in files)==['000078','000078','000079','000079']
assert sum(p.name.endswith('.up.sql') for p in files)==2
assert sum(p.name.endswith('.down.sql') for p in files)==2
f78=(m/'000078_pre_cow_notice_and_gst_integrity.up.sql').read_text()
f79=(m/'000079_pre_cow_charging_tenant_integrity.up.sql').read_text()
for path in files:
    s=path.read_text()
    assert 'DROP TABLE' not in s.upper() and 'TRUNCATE' not in s.upper()
    assert not re.search(r'(?im)^\s*(?:DELETE\s+FROM|UPDATE\s+\w+\s+SET|INSERT\s+INTO)\b',s)
    assert len(re.findall(r'\$\$',s))%2==0
    assert len(s.strip())>200
for s in [f78,f79]:
    assert re.search(r'ALTER TABLE',s)
    assert 'NOT VALID' in s and 'VALIDATE CONSTRAINT' in s
    assert 'ON DELETE RESTRICT' in s
assert 'uq_platform_notifications_platform_recipient' in f78
assert 'ON DELETE NO ACTION' in f78 and 'DEFERRABLE INITIALLY DEFERRED' in f78
assert 'ON DELETE SET NULL' not in re.sub(r'--[^\n]*','',f78)
assert 'wallets(cpo_id, customer_id, id)' in f79
assert 'FOREIGN KEY (cpo_id, customer_id, wallet_id)' in f79
assert 'FOREIGN KEY (cpo_id, customer_id, session_id)' in f79
for v,s,expected in [('078',f78,2),('079',f79,16)]:
    names=re.findall(r'ADD CONSTRAINT\s+([a-zA-Z_][\w]*)\s+FOREIGN KEY',s,re.I)
    validated=re.findall(r'VALIDATE CONSTRAINT\s+([a-zA-Z_][\w]*)',s,re.I)
    assert len(names)==expected,(v,names)
    assert Counter(names)==Counter(validated),(v,set(names)-set(validated),set(validated)-set(names))
    assert all(len(n)<=63 for n in names)
test=(root/'db'/'pre_cow_integrity_migration_integration_test.go').read_text()
assert 'TEST_DATABASE_URL' in test and 'CREATE SCHEMA' in test and 'ROLLBACK' in test
assert 'TestPreCoWNoticeAndGSTIntegrityWithPostgreSQL' in test
assert 'TestPreCoWChargingTenantIntegrityWithPostgreSQL' in test
for p in [root/'scripts'/'pre_cow_integrity_readonly_preflight.sql',root/'scripts'/'pre_cow_integrity_readonly_postflight.sql']:
    s=p.read_text();assert 'BEGIN READ ONLY;' in s and 'COMMIT;' in s
    assert not re.search(r'(?im)^\s*(?:ALTER|UPDATE|DELETE|INSERT|TRUNCATE|DROP|CREATE)\s+',s)
print('Static package checks PASS: 4 migration files, 18 FK validations, immutable data, 2 PG-gated scenarios, read-only pre/postflight.')
print('Scope: textual consistency ONLY; no PostgreSQL execution, no migration applied, no Go test run.')
