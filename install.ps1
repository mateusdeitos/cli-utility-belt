$ErrorActionPreference = "Stop"

$Repo = "mateusdeitos/cli-utility-belt"
$Binary = "belt.exe"
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\belt" }

$Arch = if ([System.Environment]::Is64BitOperatingSystem) { "amd64" } else {
    Write-Error "Unsupported architecture"
    exit 1
}

$Asset = "belt-windows-$Arch.exe"
$Url = "https://github.com/$Repo/releases/latest/download/$Asset"

Write-Host "Downloading $Asset..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Invoke-WebRequest -Uri $Url -OutFile "$InstallDir\$Binary"

# Add to PATH if not already present
$CurrentPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
if ($CurrentPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable("PATH", "$CurrentPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to PATH (restart your terminal to apply)"
}

Write-Host "Installed to $InstallDir\$Binary"
Write-Host "Run 'belt --help' to get started."
