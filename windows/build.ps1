$ErrorActionPreference = 'Stop'
Push-Location (Split-Path $PSScriptRoot)
try {
    Push-Location web
    try {
        & npm.cmd ci
        if ($LASTEXITCODE) { throw 'npm ci failed' }
        & npm.cmd run build
        if ($LASTEXITCODE) { throw 'Frontend build failed' }
    } finally { Pop-Location }
    $version = (Get-Content VERSION -Raw).Trim()
    & go build -trimpath -ldflags "-s -w -X main.version=$version" -o dist/mediarium.exe ./cmd/app
    if ($LASTEXITCODE) { throw 'Go build failed' }
} finally { Pop-Location }
