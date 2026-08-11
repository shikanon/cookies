$script:LocalAcceptanceRepoRoot = Split-Path -Parent $PSScriptRoot
$script:SeedTextAlias = "cookies.text.standard"
$script:SeedTextModel = "doubao-seed-2-0-pro-260215"

function Get-DotEnvSetting {
    param([Parameter(Mandatory = $true)][string]$Name)

    $envFile = Join-Path $script:LocalAcceptanceRepoRoot ".env"
    if (-not (Test-Path -LiteralPath $envFile)) {
        return ""
    }
    $line = Get-Content -LiteralPath $envFile |
        Where-Object { $_ -match "^\s*(?:export\s+)?$([regex]::Escape($Name))\s*=" } |
        Select-Object -Last 1
    if ($null -eq $line) {
        return ""
    }
    return (($line -split "=", 2)[1].Trim()).Trim('"').Trim("'")
}

function Get-LocalAcceptanceSetting {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [string]$Default = ""
    )

    $value = [Environment]::GetEnvironmentVariable($Name, "Process")
    if ([string]::IsNullOrWhiteSpace($value)) {
        $value = [Environment]::GetEnvironmentVariable($Name, "User")
    }
    if ([string]::IsNullOrWhiteSpace($value)) {
        $value = Get-DotEnvSetting $Name
    }
    if ([string]::IsNullOrWhiteSpace($value)) {
        $value = $Default
    }
    return $value
}

function Set-LocalAcceptanceProcessSetting {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$Value
    )

    [Environment]::SetEnvironmentVariable($Name, $Value, "Process")
}

function Assert-LocalCommand {
    param([Parameter(Mandatory = $true)][string]$Name)

    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command '$Name' was not found in PATH."
    }
}

function Initialize-LocalAcceptanceEnvironment {
    $mysqlPort = Get-LocalAcceptanceSetting "COOKIES_MYSQL_PORT" "3307"
    $defaults = [ordered]@{
        "COOKIES_ENV"                                  = "local"
        "COOKIES_HTTP_ADDR"                            = "127.0.0.1:8080"
        "COOKIES_MYSQL_PORT"                           = $mysqlPort
        "COOKIES_MYSQL_DSN"                            = "cookies:cookies_local_development_only@tcp(127.0.0.1:$mysqlPort)/cookies?parseTime=true&multiStatements=true"
        "COOKIES_BLOB_PROVIDER"                        = "filesystem"
        "COOKIES_FILESYSTEM_BLOB_ROOT"                 = ".data/blobs"
        "COOKIES_LOCAL_ORGANIZATION_ID"                = "org_local"
        "COOKIES_LOCAL_PRINCIPAL_KIND"                 = "user"
        "COOKIES_LOCAL_PRINCIPAL_ID"                   = "user_local"
        "COOKIES_LOCAL_PROJECT_ID"                     = "project_local"
        "COOKIES_LOCAL_SCOPES"                         = "project.read,project.write,assets.read,assets.write,provider.job.create,provider.text.generate,provider.vision.understand,creative.read,creative.write,strategy.read,strategy.write,strategy.confirm,strategy.review,strategy.approve,strategy.package.read"
        "COOKIES_STRATEGY_ENABLED"                     = "true"
        "COOKIES_STRATEGY_CRITIC_ENABLED"              = "true"
        "COOKIES_STRATEGY_APPROVE_ENABLED"             = "true"
        "COOKIES_STRATEGY_PACKAGE_TO_CREATIVE_ENABLED" = "true"
        "COOKIES_STRATEGY_CREATIVE_TASK_PLANNING_ENABLED" = "true"
        "COOKIES_CREATIVE_DIRECTION_PLANNING_ENABLED"    = "true"
        "COOKIES_CREATIVE_DIRECTION_PLANNER_MODEL_ALIAS" = "cookies.text.standard"
        "COOKIES_PROVIDER_ALLOW_INSECURE_HTTP"          = "true"
        "COOKIES_PROVIDER_OUTPUT_BUCKET"                = "cookies-provider-output"
    }
    foreach ($setting in $defaults.GetEnumerator()) {
        $value = Get-LocalAcceptanceSetting $setting.Key $setting.Value
        Set-LocalAcceptanceProcessSetting $setting.Key $value
    }

    $optionalSettings = @(
        "COOKIES_PROVIDER_IMAGE_ADAPTER",
        "COOKIES_PROVIDER_VIDEO_ADAPTER",
        "COOKIES_PROVIDER_AUDIO_ADAPTER",
        "COOKIES_PROVIDER_MASTER_KEY_VERSION",
        "COOKIES_PASSWORD_AUTH_ENABLED",
        "COOKIES_ADMIN_USERNAME",
        "COOKIES_ADMIN_PASSWORD",
        "COOKIES_SESSION_HOURS"
    )
    foreach ($name in $optionalSettings) {
        $value = Get-LocalAcceptanceSetting $name
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            Set-LocalAcceptanceProcessSetting $name $value
        }
    }

    $fontPath = Get-LocalAcceptanceSetting "COOKIES_CREATIVE_IMAGE_FONT_PATH"
    if ([string]::IsNullOrWhiteSpace($fontPath)) {
        $windowsRoot = [Environment]::GetEnvironmentVariable("WINDIR", "Process")
        if (-not [string]::IsNullOrWhiteSpace($windowsRoot)) {
            $fontCandidate = Join-Path $windowsRoot "Fonts\msyh.ttc"
            if (Test-Path -LiteralPath $fontCandidate) {
                $fontPath = $fontCandidate
            }
        }
    }
    if (-not [string]::IsNullOrWhiteSpace($fontPath)) {
        if (-not (Test-Path -LiteralPath $fontPath)) {
            throw "COOKIES_CREATIVE_IMAGE_FONT_PATH does not exist: $fontPath"
        }
        $fontHash = Get-LocalAcceptanceSetting "COOKIES_CREATIVE_IMAGE_FONT_SHA256"
        if ([string]::IsNullOrWhiteSpace($fontHash)) {
            $fontHash = (Get-FileHash -LiteralPath $fontPath -Algorithm SHA256).Hash.ToLowerInvariant()
        }
        Set-LocalAcceptanceProcessSetting "COOKIES_CREATIVE_IMAGE_FONT_PATH" $fontPath
        Set-LocalAcceptanceProcessSetting "COOKIES_CREATIVE_IMAGE_FONT_SHA256" $fontHash
    }

    $masterKey = Get-LocalAcceptanceSetting "COOKIES_PROVIDER_MASTER_KEY"
    if ([string]::IsNullOrWhiteSpace($masterKey)) {
        throw "COOKIES_PROVIDER_MASTER_KEY is missing. Run scripts\import-clawex-model-providers.ps1 once to import the encrypted Adapter credential."
    }
    Set-LocalAcceptanceProcessSetting "COOKIES_PROVIDER_MASTER_KEY" $masterKey

    $masterKeyVersion = Get-LocalAcceptanceSetting "COOKIES_PROVIDER_MASTER_KEY_VERSION"
    if ([string]::IsNullOrWhiteSpace($masterKeyVersion)) {
        throw "COOKIES_PROVIDER_MASTER_KEY_VERSION is missing. Run scripts\import-clawex-model-providers.ps1 once."
    }
    Set-LocalAcceptanceProcessSetting "COOKIES_PROVIDER_MASTER_KEY_VERSION" $masterKeyVersion

    # Manual acceptance always exercises the real Seed route. These values are
    # deliberately set in the process so a stale fake value cannot win.
    Set-LocalAcceptanceProcessSetting "COOKIES_PROVIDER_TEXT_ADAPTER" "adapter_gateway"
    Set-LocalAcceptanceProcessSetting "COOKIES_PROVIDER_IMAGE_ADAPTER" "adapter_gateway"
    Set-LocalAcceptanceProcessSetting "COOKIES_STRATEGY_REAL_PROVIDER_ENABLED" "true"
    Set-LocalAcceptanceProcessSetting "COOKIES_STRATEGY_TEXT_MODEL_ALIAS" $script:SeedTextAlias
    $deepReviewAlias = Get-LocalAcceptanceSetting "COOKIES_STRATEGY_DEEP_REVIEW_MODEL_ALIAS" $script:SeedTextAlias
    Set-LocalAcceptanceProcessSetting "COOKIES_STRATEGY_DEEP_REVIEW_MODEL_ALIAS" $deepReviewAlias
}

function Test-LocalHTTP {
    param([Parameter(Mandatory = $true)][string]$Url)

    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 2
        return $response.StatusCode -ge 200 -and $response.StatusCode -lt 300
    }
    catch {
        return $false
    }
}

function Test-LocalListeningPort {
    param([Parameter(Mandatory = $true)][int]$Port)

    return $null -ne (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1)
}

function Assert-SeedTextRoute {
    $mysqlUser = Get-LocalAcceptanceSetting "COOKIES_MYSQL_USER" "cookies"
    $mysqlPassword = Get-LocalAcceptanceSetting "COOKIES_MYSQL_PASSWORD" "cookies_local_development_only"
    $mysqlDatabase = Get-LocalAcceptanceSetting "COOKIES_MYSQL_DATABASE" "cookies"
    $query = @"
SELECT r.model_alias, rr.upstream_model, r.status, c.connection_type, c.status, COUNT(pc.id)
FROM provider_model_routes r
JOIN provider_model_route_revisions rr
  ON rr.id = r.current_revision_id AND rr.route_id = r.id
JOIN provider_connections c
  ON c.id = rr.connection_id AND c.current_revision_id = rr.connection_revision_id
LEFT JOIN provider_credentials pc
  ON pc.connection_id = c.id AND pc.status = 'active'
WHERE r.organization_id IS NULL
  AND r.capability = 'text.generate'
  AND r.model_alias = '$script:SeedTextAlias'
GROUP BY r.model_alias, rr.upstream_model, r.status, c.connection_type, c.status;
"@
    $result = @(& docker compose exec -T -e "MYSQL_PWD=$mysqlPassword" mysql mysql `
        -N -B "-u$mysqlUser" $mysqlDatabase -e $query 2>&1)
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to inspect the Seed model route: $($result -join [Environment]::NewLine)"
    }
    $row = $result | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -First 1
    if ([string]::IsNullOrWhiteSpace($row)) {
        throw "The '$script:SeedTextAlias' route is missing. Run scripts\import-clawex-model-providers.ps1 once."
    }
    $fields = $row -split "`t"
    if ($fields.Count -ne 6 -or
        $fields[0] -ne $script:SeedTextAlias -or
        $fields[1] -ne $script:SeedTextModel -or
        $fields[2] -ne "enabled" -or
        $fields[3] -notin @("adapter_gateway", "ark") -or
        $fields[4] -ne "enabled" -or
        [int]$fields[5] -lt 1) {
        throw "The '$script:SeedTextAlias' route is not ready for $script:SeedTextModel. Configure an active Adapter or Ark provider connection."
    }
    Write-Host "Seed text route verified: $script:SeedTextAlias -> $script:SeedTextModel"
}
