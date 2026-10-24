[CmdletBinding()]
param([string]$InstallDir = 'C:\Program Files\Mediarium')
$ErrorActionPreference = 'Stop'
$toolsDir = Join-Path $InstallDir 'tools'
New-Item -ItemType Directory -Force -Path $toolsDir | Out-Null
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('mediarium-tools-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $scratch | Out-Null
# Downloads are pinned to upstream release assets and their published SHA-256 digests.
$archives = @(
    @('https://github.com/ip7z/7zip/releases/download/26.04/7z2604-extra.7z', '7zip.7z', 'dc4b11d3399db18b063630137145f5585d8f7ac847bf3639bd1185d7d1f7cee0'),
    @('https://github.com/Parchive/par2cmdline/releases/download/v1.4.0/par2cmdline-1.4.0-win-x64.zip', 'par2.zip', '200130143dba1e12f9d1be567d5a446e785d2093e5d735225d8ad0539125b71c')
)
foreach ($archive in $archives) {
    $path = Join-Path $scratch $archive[1]
    Invoke-WebRequest -UseBasicParsing -Uri $archive[0] -OutFile $path
    if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $archive[2]) { throw "Checksum failed for $($archive[1])" }
    $target = Join-Path $scratch ([IO.Path]::GetFileNameWithoutExtension($archive[1]))
    New-Item -ItemType Directory -Path $target | Out-Null
    if ($archive[1] -like '*.zip') {
        Expand-Archive -LiteralPath $path -DestinationPath $target
    } else {
        $sevenZip = Get-Command 7z.exe -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Source
        if (!$sevenZip -and (Test-Path "${env:ProgramFiles}\7-Zip\7z.exe")) { $sevenZip = "${env:ProgramFiles}\7-Zip\7z.exe" }
        if ($sevenZip) {
            & $sevenZip x -y "-o$target" $path
        } else {
            # Windows 11's libarchive supports this; older Windows Server
            # tar builds lack LZMA. CI has 7-Zip preinstalled.
            & tar.exe -xf $path -C $target
        }
        if ($LASTEXITCODE) { throw "Extraction failed for $($archive[1]); install 7-Zip if tar lacks LZMA support." }
    }
}
Copy-Item -LiteralPath (Join-Path $scratch '7zip\x64\7za.exe') -Destination (Join-Path $toolsDir '7z.exe') -Force
Copy-Item -LiteralPath (Join-Path $scratch '7zip\License.txt') -Destination (Join-Path $toolsDir '7zip-License.txt') -Force
Get-ChildItem (Join-Path $scratch 'par2') -Recurse -File | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $toolsDir -Force }
& (Join-Path $toolsDir 'par2.exe') -V
if ($LASTEXITCODE) { throw 'PAR2 tool could not run' }
Write-Output "Tools installed in $toolsDir."
