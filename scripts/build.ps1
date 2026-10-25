$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$env:GOCACHE = Join-Path $root '.cache/go-build'
$egglog = if ($env:EGGLOG_BIN) { $env:EGGLOG_BIN } else { Join-Path $root '.tools/bin/egglog.exe' }
if (-not (Test-Path -LiteralPath $egglog)) { throw "egglog 0.4.0 missing: set EGGLOG_BIN or install .tools/bin/egglog.exe" }
$version = & $egglog --version
if ($LASTEXITCODE -ne 0 -or -not $version.StartsWith('egglog 0.4.0')) { throw "egglog 0.4.0 required; got: $version" }
New-Item -ItemType Directory -Path (Join-Path $root 'bin') -Force | Out-Null
go build -o (Join-Path $root 'bin/egglog-planner.exe') ./cmd/egglog-planner
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Output "Built bin/egglog-planner.exe with $version"
