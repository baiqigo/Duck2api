param(
  [string]$Version = "latest",
  [string]$InstallDir = "$env:LOCALAPPDATA\Duck2API"
)

$ErrorActionPreference = "Stop"
$repo = if ($env:DUCK2API_REPO) { $env:DUCK2API_REPO } else { "baiqigo/Duck2api" }
$asset = "duck2api-windows-amd64.zip"
if ($Version -eq "latest") {
  $base = "https://github.com/$repo/releases/latest/download"
} else {
  $base = "https://github.com/$repo/releases/download/$Version"
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$zip = Join-Path $env:TEMP $asset
Write-Host "Downloading $base/$asset"
Invoke-WebRequest -Uri "$base/$asset" -OutFile $zip

$checksums = Invoke-WebRequest -Uri "$base/checksums.txt" -UseBasicParsing
$checksumText = [System.Text.Encoding]::UTF8.GetString($checksums.Content)
$line = ($checksumText -split "`r?`n" | Where-Object { $_ -match "  $([regex]::Escape($asset))$" } | Select-Object -First 1)
if (-not $line) { throw "checksum for $asset not found" }
$expected = ($line -split '\s+')[0]
$actual = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash
if ($actual -ne $expected) { throw "checksum mismatch for $asset" }

Expand-Archive -Path $zip -DestinationPath $InstallDir -Force
Remove-Item $zip
Write-Host "Installed to $InstallDir\duck2api.exe"
Write-Host "Add $InstallDir to PATH, then run: duck2api"