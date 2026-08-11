param(
    [string]$Model = "doubao-seed-2-0-pro-260215"
)

# Reuses the already encrypted company Adapter credential. Seed-2-pro text
# routes must not silently switch to a direct Ark connection: the two
# transports have different credential ownership and operational contracts.
# No token is read from command-line arguments, written to .env, or embedded
# in SQL.

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $repoRoot ".env"

function Get-DotEnvValue([string]$Key) {
    $line = Get-Content -LiteralPath $envFile |
        Where-Object { $_ -match "^\s*$([regex]::Escape($Key))\s*=" } |
        Select-Object -First 1
    if ($null -eq $line) { return "" }
    return (($line -split "=", 2)[1].Trim()).Trim('"').Trim("'")
}

function Set-DotEnvValue([string]$Key, [string]$Value) {
    $content = @(Get-Content -LiteralPath $envFile)
    $pattern = "^\s*$([regex]::Escape($Key))\s*="
    $updated = $false
    $next = foreach ($line in $content) {
        if ($line -match $pattern) {
            "$Key=$Value"
            $updated = $true
        }
        else {
            $line
        }
    }
    if (-not $updated) { $next += "$Key=$Value" }
    Set-Content -LiteralPath $envFile -Value $next -Encoding utf8
}

Push-Location $repoRoot
try {
    if (-not (Test-Path -LiteralPath $envFile)) {
        throw "Missing $envFile. Copy .env.example to .env first."
    }
    $mysqlPassword = Get-DotEnvValue "COOKIES_MYSQL_PASSWORD"
    if ([string]::IsNullOrWhiteSpace($mysqlPassword)) {
        $mysqlPassword = "cookies_local_development_only"
    }
    & docker compose up -d mysql | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Unable to start local MySQL." }
    & go run ./cmd/cookies-migrate | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Database migrations failed." }

    $containerID = (& docker compose ps -q mysql).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($containerID)) {
        throw "The local MySQL container is not running."
    }
    $routeRow = (& docker exec -e "MYSQL_PWD=$mysqlPassword" $containerID mysql -N -B -u cookies cookies -e @"
SELECT r.id, c.id, c.current_revision_id, c.connection_type
FROM provider_model_routes r
JOIN provider_connections c ON c.status = 'enabled' AND c.current_revision_id IS NOT NULL
JOIN provider_credentials pc ON pc.connection_id = c.id
  AND pc.status = 'active'
  AND pc.active_from <= UTC_TIMESTAMP(6)
  AND (pc.active_until IS NULL OR pc.active_until > UTC_TIMESTAMP(6))
WHERE r.organization_id IS NULL
  AND r.capability = 'text.generate'
  AND r.model_alias = 'cookies.text.standard'
  AND r.status = 'enabled'
  AND c.connection_type = 'adapter_gateway'
ORDER BY pc.credential_version DESC
LIMIT 1;
"@).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($routeRow)) {
        throw "No enabled Adapter gateway credential exists for cookies.text.standard. Configure or rotate the company Adapter credential first."
    }
    $routeFields = @($routeRow -split "`t")
    if ($routeFields.Count -ne 4) {
        throw "The current cookies.text.standard route is incomplete."
    }
    $routeID = $routeFields[0]
    $connectionID = $routeFields[1]
    $connectionRevisionID = $routeFields[2]
    $connectionType = $routeFields[3]
    $routeRevisionID = "${routeID}_seed2_standard_r1"
    $deepRouteResult = & docker exec -e "MYSQL_PWD=$mysqlPassword" $containerID mysql -N -B -u cookies cookies -e "SELECT id FROM provider_model_routes WHERE organization_id IS NULL AND capability='text.generate' AND model_alias='cookies.text.deep_review' LIMIT 1"
    if ($LASTEXITCODE -ne 0) { throw "Could not inspect the existing deep-reasoning route." }
    $deepRouteID = if ($null -eq $deepRouteResult) { "" } else { ([string]$deepRouteResult).Trim() }
    if ([string]::IsNullOrWhiteSpace($deepRouteID)) { $deepRouteID = "route_cookies_text_deep_review" }
    $deepRouteRevisionID = "${deepRouteID}_seed2_thinking_r1"
    $sql = @"
START TRANSACTION;
UPDATE provider_model_routes SET current_revision_id = NULL WHERE id IN ('$routeID', '$deepRouteID');
DELETE FROM provider_model_route_revisions WHERE route_id IN ('$routeID', '$deepRouteID');
UPDATE provider_model_routes
SET capability = 'text.generate', model_alias = 'cookies.text.standard', status = 'enabled'
WHERE id = '$routeID';
INSERT INTO provider_model_routes (id, organization_id, capability, model_alias, current_revision_id, status)
VALUES ('$deepRouteID', NULL, 'text.generate', 'cookies.text.deep_review', NULL, 'enabled')
ON DUPLICATE KEY UPDATE model_alias = VALUES(model_alias), status = VALUES(status);
INSERT INTO provider_model_route_revisions (
  id, route_id, revision_number, connection_id, connection_revision_id, upstream_model, constraints_json
) VALUES (
  '$routeRevisionID', '$routeID', 1, '$connectionID', '$connectionRevisionID', '$Model',
  JSON_OBJECT(
    'endpoint', '/v1/chat/completions',
    'source_provider', '$connectionType',
    'text_response_mode', 'prompt_json',
    'max_output_tokens', 8192,
    'output_token_parameter', 'max_tokens',
    'temperature', 0.2,
    'thinking_mode', 'disabled'
  )
);
UPDATE provider_model_routes SET current_revision_id = '$routeRevisionID' WHERE id = '$routeID';
INSERT INTO provider_model_route_revisions (
  id, route_id, revision_number, connection_id, connection_revision_id, upstream_model, constraints_json
) VALUES (
  '$deepRouteRevisionID', '$deepRouteID', 1, '$connectionID', '$connectionRevisionID', '$Model',
  JSON_OBJECT(
    'endpoint', '/v1/chat/completions',
    'source_provider', '$connectionType',
    'text_response_mode', 'prompt_json',
    'max_output_tokens', 16384,
    'output_token_parameter', 'max_tokens',
    'temperature', 0.2,
    'thinking_mode', 'enabled'
  )
);
UPDATE provider_model_routes SET current_revision_id = '$deepRouteRevisionID' WHERE id = '$deepRouteID';
COMMIT;
"@
    & docker exec -i -e "MYSQL_PWD=$mysqlPassword" $containerID mysql -u cookies cookies -e $sql
    if ($LASTEXITCODE -ne 0) { throw "Saving the Seed-2-pro standard and deep-thinking routes failed." }

    Set-DotEnvValue "COOKIES_PROVIDER_TEXT_ADAPTER" "adapter_gateway"
    Set-DotEnvValue "COOKIES_STRATEGY_REAL_PROVIDER_ENABLED" "true"
    Set-DotEnvValue "COOKIES_STRATEGY_TEXT_MODEL_ALIAS" "cookies.text.standard"
    Set-DotEnvValue "COOKIES_STRATEGY_DEEP_REVIEW_MODEL_ALIAS" "cookies.text.deep_review"
    Write-Output "Seed-2-pro standard route: cookies.text.standard via Adapter gateway (thinking disabled)."
    Write-Output "Seed-2-pro deep route:     cookies.text.deep_review (thinking enabled)."
}
finally {
    Pop-Location
}
