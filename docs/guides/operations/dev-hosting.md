# Development VPS Hosting

## Deployment Shape

The development deployment for `dev-evcmsnew.transev.site` uses:

```text
HTTPS client
→ Caddy on ports 80/443
→ EV CMS on 127.0.0.1:18080
→ PostgreSQL database devevcmsnewdb on 127.0.0.1:5432
```

The application is managed by `evcmsnew-dev.service`. The service runs the
repository-local `builds/evcmsnew` binary with the ignored `.env` as its
environment file. The HTTP application is deliberately not exposed directly
on a public interface.

The SMTP outbox worker is part of the application process. It must not be
started as a second systemd service because that would create an unnecessary
duplicate worker.

The active deployment was updated on August 3, 2026 to source revision
`9760523`. It has migrations one through eleven and the current 70-operation
API. GSTIN and complete address identity are database-required for CPOs, the
authenticated platform slug-availability route is live, and known uniqueness
races return field- or relationship-specific conflict codes. The eleven
retired commercial prototype tables remain recoverable in the
`retired_commercial` schema.

## Files and Ownership

| Purpose | Location |
|---|---|
| Repository and working directory | `/root/ev-cms-backend-new` |
| Ignored deployment environment | `/root/ev-cms-backend-new/.env` |
| Compiled binary | `/root/ev-cms-backend-new/builds/evcmsnew` |
| systemd unit | `/etc/systemd/system/evcmsnew-dev.service` |
| Caddy configuration | `/etc/caddy/Caddyfile` |
| PostgreSQL database | `devevcmsnewdb`, owned by `postgres` |

The `.env` file must remain mode `0600`. It contains the superadmin bootstrap
password, SMTP password, database password, and encryption material and must
never be committed or printed.

## Configuration

The deployment copies `.env.example` to `.env`, then overrides:

- `DATABASE_URL` to use `devevcmsnewdb`;
- `HTTP_ADDR=127.0.0.1:18080`;
- the initial superadmin identity;
- five independently generated 32-byte base64 cryptographic keys.

`DATABASE_URL` and `SMTP_PASSWORD` contain deployment secrets only in the
ignored environment file. The service is enabled and active, all eleven forward
migrations are recorded, and startup idempotently retained the configured
platform superadmin.

The deployment environment explicitly records `API_DOCS_ENABLED=true` and the
documented platform event, realtime, worker-staleness, and maintenance defaults.
`PLATFORM_WORKER_STALE_AFTER=2m` intentionally exceeds the `1m` maintenance
heartbeat so readiness does not oscillate between worker updates.

`CORS_ALLOW_ALL=true` remains enabled for the current cross-origin development
frontend integration. This is not the intended permanent production CORS
policy.

## Build and Activation

Build without starting the service:

```bash
cd /root/ev-cms-backend-new
mkdir -p builds
GOCACHE=/tmp/evcmsnew-go-cache go build -trimpath -o builds/evcmsnew .
```

After any future secret or binary update, validate the environment without
printing secret values. Restart through `rehost-evcmsnew`, or activate a
deliberately disabled instance with:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now evcmsnew-dev.service
sudo systemctl status evcmsnew-dev.service --no-pager
curl --fail --silent --show-error https://dev-evcmsnew.transev.site/health/live
curl --fail --silent --show-error https://dev-evcmsnew.transev.site/health/ready
```

Application startup remains an idempotent migration and bootstrap boundary. A
healthy `/health/live` alone is insufficient; `/health/ready` must also report
database readiness. The active deployment passed both checks over loopback and
public HTTPS. One real platform-login OTP reached `SENT` through Hostinger on
its first delivery attempt.

## Rehosting and Logs

After rebuilding `builds/evcmsnew`, an interactive root shell can use:

```bash
rehost-evcmsnew
show-logs evcmsnew-dev
```

`rehost-evcmsnew` delegates to the shared `/usr/local/bin/rehost-service`
handler: it stops the service, reloads systemd, waits with a skippable
countdown, starts it, and tails its journal. It does not rebuild the binary.

Source revisions containing the request logger emit one JSON
`http_request_completed` line to stdout after every Gin request finishes. Use
the response `X-Request-ID` to locate the record. The schema and mandatory
content exclusions are defined in
`docs/contracts/internal/http-request-logging.md`. Long-lived SSE requests are
recorded when they disconnect. A recovered panic first emits a correlated safe
JSON stack diagnostic without Gin's request dump or the panic value. The
currently deployed `afd90f5` binary predates this logger until a later
authorized release replaces it.

For a developer diagnostic session, set `LOG_LEVEL=DEBUG` in the ignored
deployment environment and rehost. This adds request-start and handled-error
component/type events under the same request ID. Return it to `INFO` for concise
normal operation. Neither mode logs payloads, raw URLs/queries, credentials,
personal fields, or raw errors.

## Safe Diagnostics

```bash
systemctl status evcmsnew-dev.service --no-pager
journalctl -u evcmsnew-dev.service -n 120 --no-pager
journalctl -u evcmsnew-dev.service --since '10 minutes ago' --no-pager
ss -ltnp | grep ':18080'
sudo -u postgres psql -X -Atqc \
  "SELECT datname, pg_get_userbyid(datdba) FROM pg_database WHERE datname = 'devevcmsnewdb';"
caddy validate --config /etc/caddy/Caddyfile
```

Do not paste `.env`, tokens, OTPs, or journal lines containing sensitive
request data into tickets or chat.
