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
    'docs/guides/concepts/identity-tenancy-and-application-identity.md',
    'docs/guides/concepts/superadmin-control-plane.md',
    'docs/guides/workflows/cpo-onboarding.md',
    'docs/guides/workflows/app-user-authentication.md',
    'docs/guides/troubleshooting/authentication-and-mail.md',
    'docs/integrations/smtp-mail-delivery.md',
    'docs/integrations/razorpay-credential-storage.md',
    'docs/integrations/ocpp-hal-boundary.md',
    'docs/contracts/api/administrative-http-api.md',
    'docs/contracts/internal/mail-outbox.md',
    'docs/contracts/realtime/platform-events.md',
    'docs/contracts/configuration.md',
    'docs/contracts/openapi/openapi.yaml',
    'docs/decisions/0004-openapi-and-embedded-api-explorer.md',
    'docs/decisions/0005-cpo-scoped-customer-signup.md',
    'docs/decisions/0006-customer-session-plane.md',
    'docs/decisions/0007-complete-superadmin-control-plane.md',
    'docs/decisions/0008-manual-cpo-access-without-commercial-management.md',
    'docs/plans/api-documentation-and-openapi.md',
    'docs/plans/customer-signup.md',
    'docs/plans/customer-authentication.md',
    'docs/plans/superadmin-control-plane.md'
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

$routeContract = Get-Content -Raw -LiteralPath (
    Join-Path $repositoryRoot 'docs/contracts/api/administrative-http-api.md'
)
$requiredRoutes = @(
    '/api/v1/auth/login',
    '/api/v1/app/auth/signup',
    '/api/v1/app/auth/login',
    '/api/v1/app/auth/me',
    '/api/v1/auth/password/change',
    '/api/v1/platform/cpos',
    '/api/v1/platform/events',
    '/api/v1/platform/realtime/stream',
    '/api/v1/platform/audit-logs',
    '/api/v1/platform/workers',
    '/api/v1/cpo/integrations',
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
    '/api/v1/platform/plans',
    '/subscription',
    '/entitlement-overrides/',
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
