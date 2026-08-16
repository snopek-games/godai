# Installs the godai CLI on Windows:
#
#   irm https://godai.sh/install.ps1 | iex
#
# (or, equivalently, the raw file from the repository:
# https://gitlab.com/snopek-games/godai/-/raw/main/packaging/install/install.ps1)
#
# Configuration (environment variables, since `iex` can't pass parameters):
#   GODAI_VERSION         version to install, without the leading "v" (default: latest release)
#   GODAI_INSTALL_DIR     where to put the binary (default: %LOCALAPPDATA%\Programs\godai)
#   GODAI_NO_MODIFY_PATH  if non-empty, never edit the user PATH
#   GODAI_DOWNLOAD_BASE   override the release download base URL (used by CI
#                         to test this script against a local server)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$ProjectApi = 'https://gitlab.com/api/v4/projects/snopek-games%2Fgodai'
$BaseUrl = if ($env:GODAI_DOWNLOAD_BASE) { $env:GODAI_DOWNLOAD_BASE } else { "$ProjectApi/packages/generic/release-packages" }
$InstallDir = if ($env:GODAI_INSTALL_DIR) { $env:GODAI_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\godai' }

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'x86_64' }
    'ARM64' { 'arm64' }
    default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

$Version = $env:GODAI_VERSION
if (-not $Version) {
    $Version = (Invoke-RestMethod "$ProjectApi/releases/permalink/latest").tag_name -replace '^v', ''
}

$Asset = "godai-cli-windows-$Arch-v$Version"

$TmpDir = Join-Path ([IO.Path]::GetTempPath()) "godai-install-$([IO.Path]::GetRandomFileName())"
New-Item -ItemType Directory -Path $TmpDir | Out-Null
try {
    Write-Host "Downloading godai v$Version (windows-$Arch)..."
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/v$Version/$Asset.zip" -OutFile "$TmpDir\$Asset.zip"
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/v$Version/checksums-v$Version.txt" -OutFile "$TmpDir\checksums.txt"

    $Expected = $null
    foreach ($Line in Get-Content "$TmpDir\checksums.txt") {
        $Parts = -split $Line
        if ($Parts.Count -eq 2 -and $Parts[1] -eq "$Asset.zip") {
            $Expected = $Parts[0]
        }
    }
    if (-not $Expected) {
        throw "No checksum for $Asset.zip in the release's checksums file"
    }
    $Actual = (Get-FileHash -Algorithm SHA256 "$TmpDir\$Asset.zip").Hash
    if ($Actual.ToLower() -ne $Expected.ToLower()) {
        throw "Checksum mismatch for $Asset.zip (expected $Expected, got $Actual)"
    }

    Expand-Archive -Path "$TmpDir\$Asset.zip" -DestinationPath $TmpDir
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path "$TmpDir\$Asset\godai.exe" -Destination (Join-Path $InstallDir 'godai.exe') -Force
} finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}

Write-Host "Installed godai v$Version to $InstallDir\godai.exe"

if ($env:OS -eq 'Windows_NT' -and -not $env:GODAI_NO_MODIFY_PATH) {
    $UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($UserPath -split ';') -notcontains $InstallDir) {
        $NewPath = if ($UserPath) { "$UserPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable('Path', $NewPath, 'User')
        Write-Host "Added $InstallDir to your PATH - open a new terminal for it to take effect."
    }
    $env:Path = "$env:Path;$InstallDir"
}

Write-Host 'Then run "godai --help" to get started, and "godai self-update" to update later.'
