[CmdletBinding()]
param([switch]$Windowed)
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
    $flags = "-s -w -X main.version=$version"
    if ($Windowed) { $flags += ' -H windowsgui' }
    $previousCGO = $env:CGO_ENABLED
    try {
        $env:CGO_ENABLED = '0'
        & go build -trimpath -ldflags $flags -o dist/mediarium.exe ./cmd/app
    } finally { $env:CGO_ENABLED = $previousCGO }
    if ($LASTEXITCODE) { throw 'Go build failed' }
} finally { Pop-Location }
