[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$requiredFiles = @(
    'docs/README.md',
    'docs/DEVELOPMENT_PLAN.md',
    'docs/PROJECT_STATE.md',
    'docs/AI_CHANGELOG.md',
    'docs/AUTHENTICATION.md',
    'docs/CPO_ADMINISTRATION.md',
    'docs/SUPERADMIN_FRONTEND_HANDOFF.md',
    'docs/CPO_BACKEND_AGENT_HANDOFF.md',
    'docs/guides/concepts/identity-tenancy-and-application-identity.md',
    'docs/guides/concepts/superadmin-control-plane.md',
    'docs/guides/workflows/cpo-onboarding.md',
    'docs/guides/workflows/cpo-admin-network-configuration.md',
    'docs/guides/workflows/app-user-authentication.md',
    'docs/guides/troubleshooting/authentication-and-mail.md',
    'docs/integrations/smtp-mail-delivery.md',
    'docs/integrations/razorpay-credential-storage.md',
    'docs/integrations/ocpp-hal-boundary.md',
    'docs/contracts/api/administrative-http-api.md',
    'docs/contracts/internal/mail-outbox.md',
    'docs/contracts/internal/http-request-logging.md',
    'docs/contracts/realtime/platform-events.md',
    'docs/contracts/configuration.md',
    'docs/contracts/openapi/openapi.yaml',
    'docs/decisions/0004-openapi-and-embedded-api-explorer.md',
    'docs/decisions/0005-cpo-scoped-customer-signup.md',
    'docs/decisions/0006-customer-session-plane.md',
    'docs/decisions/0007-complete-superadmin-control-plane.md',
    'docs/decisions/0008-manual-cpo-access-without-commercial-management.md',
    'docs/decisions/0009-admin-only-cpo-authority.md',
    'docs/decisions/0010-required-cpo-registration-identity.md',
    'docs/decisions/0011-safe-http-request-observability.md',
    'docs/decisions/0012-manual-platform-subscriptions-without-provider.md',
    'docs/plans/api-documentation-and-openapi.md',
    'docs/plans/customer-signup.md',
    'docs/plans/customer-authentication.md',
    'docs/plans/superadmin-control-plane.md',
    'docs/plans/cpo-admin-network-configuration.md',
    'docs/plans/manual-platform-subscriptions.md',
    'docs/contracts/api/manual-subscriptions.md'
)

$missing = @(
    foreach ($relativePath in $requiredFiles) {
        if (-not (Test-Path -LiteralPath (Join-Path $repositoryRoot $relativePath) -PathType Leaf)) {
            $relativePath
        }
    }
)
if ($missing.Count -gt 0) {
    throw "Required documentation is missing: $($missing -join ', ')"
}

$cpoAgentHandoff = Get-Content -Raw -LiteralPath (
    Join-Path $repositoryRoot 'docs/CPO_BACKEND_AGENT_HANDOFF.md'
)
$requiredCPOAgentRules = @(
    'Current callable CPO staff authority is `ADMIN` only.',
    'The presence of a table or Go model does not mean its workflow exists.',
    '`src/cpo/repository.go` is currently an empty package file.',
    'Fifteen migrations are already deployment history.',
    'do not embed or copy the HAL into this process',
    'Treat `main` and `anubhab-work` as the authoritative lines'
)
foreach ($rule in $requiredCPOAgentRules) {
    if (-not $cpoAgentHandoff.Contains($rule)) {
        throw "CPO backend agent handoff is missing required rule: $rule"
    }
}

$superadminFEHandoff = Get-Content -Raw -LiteralPath (
    Join-Path $repositoryRoot 'docs/SUPERADMIN_FRONTEND_HANDOFF.md'
)
$requiredSuperadminFERules = @(
    'Send `scope: "PLATFORM"`; omit `cpo_id`',
    'Use `fetch()` streaming, not native `EventSource`',
    'collect the recovery ID, code, and new password',
    'welcome job is rejected before the CPO transaction commits',
    '`platform.cpo.primary_admin_changed`',
    'Manual subscriptions',
    'Platform-admin governance',
    'Generic mail operations',
    'Notifications/announcements',
    'Platform overview aggregates',
    'SuperAdmin is not a CPO ADMIN',
    '`available=true` does not reserve it',
    'GSTIN and every address field are required'
)
foreach ($rule in $requiredSuperadminFERules) {
    if (-not $superadminFEHandoff.Contains($rule)) {
        throw "SuperAdmin frontend handoff is missing required rule: $rule"
    }
}

$requestLogContract = Get-Content -Raw -LiteralPath (
    Join-Path $repositoryRoot 'docs/contracts/internal/http-request-logging.md'
)
$requiredRequestLogRules = @(
    '`http_request_completed`',
    '`http_panic_recovered`',
    '`http_request_started`',
    '`http_error_handled`',
    '`LOG_LEVEL=DEBUG`',
    '`X-Request-ID`',
    '`error_code`',
    '## Developer Logging Rules',
    '`middleware.RequestID(ctx)`',
    'request or response bodies',
    'Trust `X-Forwarded-For` only from a loopback peer'
)
foreach ($rule in $requiredRequestLogRules) {
    if (-not $requestLogContract.Contains($rule)) {
        throw "HTTP request logging contract is missing required rule: $rule"
    }
}

$configurationContract = Get-Content -Raw -LiteralPath (
    Join-Path $repositoryRoot 'docs/contracts/configuration.md'
)
foreach ($rule in @('`LOG_LEVEL`', '`INFO`', '`DEBUG`')) {
    if (-not $configurationContract.Contains($rule)) {
        throw "Configuration contract is missing request-log level rule: $rule"
    }
}

$mailOutboxContract = Get-Content -Raw -LiteralPath (
    Join-Path $repositoryRoot 'docs/contracts/internal/mail-outbox.md'
)
foreach ($rule in @('canonical enqueue operation', '`challenge_id`')) {
    if (-not $mailOutboxContract.Contains($rule)) {
        throw "Mail outbox contract is missing OTP forwarding rule: $rule"
    }
}

$routeContract = Get-Content -Raw -LiteralPath (
    Join-Path $repositoryRoot 'docs/contracts/api/administrative-http-api.md'
)
$requiredRoutes = @(
    '/api/v1/auth/login',
    '/api/v1/app/auth/signup',
    '/api/v1/app/auth/login',
    '/api/v1/app/me',
    '/api/v1/app/profile',
    '/api/v1/auth/password/change',
    '/api/v1/platform/cpos',
    '/api/v1/platform/cpos/slug-availability',
    '/api/v1/platform/cpos/{cpo_id}/profile',
    '/api/v1/platform/cpos/{cpo_id}/primary-admin',
    '/api/v1/platform/cpos/{cpo_id}/primary-admin/resend-onboarding',
    '/api/v1/platform/cpos/{cpo_id}/administrative-sessions/revoke',
    '/api/v1/platform/administrators',
    '/api/v1/platform/security/locked-identities',
    '/api/v1/platform/security/users/{user_id}/sessions/revoke',
    '/api/v1/platform/mail/jobs',
    '/api/v1/platform/mail/jobs/{job_id}/retry',
    '/api/v1/platform/mail/reconcile',
    '/api/v1/platform/announcements',
    '/api/v1/platform/notifications',
    '/api/v1/platform/overview',
    '/api/v1/platform/status',
    '/api/v1/platform/events',
    '/api/v1/platform/realtime/stream',
    '/api/v1/platform/audit-logs',
    '/api/v1/platform/workers',
    '/api/v1/platform/plans',
    '/api/v1/platform/cpos/{cpo_id}/subscription',
    '/api/v1/cpo/integrations',
    '/api/v1/cpo/organization',
    '/api/v1/cpo/notifications',
    '/api/v1/cpo/notifications/{notification_id}/read',
    '/api/v1/cpo/admin/profile',
    '/api/v1/cpo/users/{user_id}',
    '/api/v1/cpo/hubs',
    '/api/v1/cpo/hubs/{hub_id}/chargers',
    '/api/v1/cpo/chargers',
    '/api/v1/cpo/gsts',
    '/api/v1/cpo/tariffs',
    '/docs/',
    '/openapi.yaml'
)
foreach ($route in $requiredRoutes) {
    if (-not $routeContract.Contains($route)) {
        throw "Administrative API contract does not contain route $route"
    }
}

$openAPI = Get-Content -Raw -LiteralPath (
    Join-Path $repositoryRoot 'docs/contracts/openapi/openapi.yaml'
)
$retiredRoutes = @(
    '/billing-account',
    '/invoices',
    '/payments',
    '/billing-timeline'
)
foreach ($route in $retiredRoutes) {
    if ($openAPI.Contains($route)) {
        throw "Retired commercial route remains in OpenAPI: $route"
    }
}

$operationCount = ([regex]::Matches($openAPI, '(?m)^\s{6}operationId:\s+')).Count
if ($operationCount -ne 137) {
    throw "OpenAPI contains $operationCount operations; expected 137."
}

if ($openAPI.Contains('/api/v1/cpo/profile')) {
    throw 'Tenant-side CPO organization profile route remains in OpenAPI.'
}

$requiredCPOEvents = @(
    'platform.cpo.profile_updated',
    'platform.cpo.primary_admin_changed',
    'platform.cpo.primary_admin_onboarding_resent',
    'platform.cpo.admin_sessions_revoked'
)
$realtimeContract = Get-Content -Raw -LiteralPath (
    Join-Path $repositoryRoot 'docs/contracts/realtime/platform-events.md'
)
foreach ($eventName in $requiredCPOEvents) {
    if (-not $realtimeContract.Contains($eventName)) {
        throw "Realtime contract does not contain CPO event $eventName"
    }
}

foreach ($eventName in @(
    'platform.subscription.issued',
    'platform.subscription.renewed',
    'platform.admin.granted',
    'platform.admin.activated',
    'platform.admin.deactivated',
    'platform.security.identity_unlocked',
    'platform.security.sessions_revoked',
    'platform.mail.job_retried',
    'platform.mail.job_canceled',
    'platform.mail.jobs_reconciled',
    'platform.mail.jobs_retained',
    'platform.announcement.created'
)) {
    if (-not $realtimeContract.Contains($eventName)) {
        throw "Realtime contract does not contain subscription event $eventName"
    }
}

$legacyName = 'SMTP_' + 'TLS_MODE'
$scanFiles = Get-ChildItem -LiteralPath $repositoryRoot -Recurse -File |
    Where-Object {
        $_.FullName -notlike "$repositoryRoot\.git\*" -and
        $_.FullName -notlike "$repositoryRoot\.local\*" -and
        $_.FullName -ne (
            Join-Path $repositoryRoot 'docs/contracts/configuration.md'
        ) -and
        $_.FullName -ne $PSCommandPath
    }
$legacyReferences = @(
    $scanFiles | Select-String -SimpleMatch $legacyName
)
if ($legacyReferences.Count -gt 0) {
    throw "Removed configuration name remains outside its migration notice."
}

Write-Host 'Documentation contract verification passed.'
