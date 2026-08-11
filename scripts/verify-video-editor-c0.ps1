$ErrorActionPreference = 'Stop'

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
docker build --target verifier -f (Join-Path $repositoryRoot 'deployments/render-worker/Dockerfile') -t cookies-render-verifier:c0 $repositoryRoot
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

docker run --rm --volume "${repositoryRoot}:/workspace" --workdir /workspace cookies-render-verifier:c0 sh scripts/verify-video-editor-c0.sh
exit $LASTEXITCODE
