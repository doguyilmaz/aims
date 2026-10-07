# Installs the aims binary from a GitHub release.
#
#   irm https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.ps1 | iex
#
# $env:AIMS_VERSION   release tag to install (default: the latest)
# $env:AIMS_BIN_DIR   where to put aims.exe (default: %LOCALAPPDATA%\Programs\aims)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$repo = 'doguyilmaz/aims'
$binDir = if ($env:AIMS_BIN_DIR) { $env:AIMS_BIN_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\aims' }

if ([Environment]::Is64BitOperatingSystem -eq $false) { throw 'aims needs 64-bit Windows' }
$arch = 'amd64'

$version = $env:AIMS_VERSION
if (-not $version) {
    # The latest release page redirects to its tag; no API token needed.
    $req = [Net.HttpWebRequest]::Create("https://github.com/$repo/releases/latest")
    $req.AllowAutoRedirect = $false
    $resp = $req.GetResponse()
    $version = $resp.Headers['Location'] -replace '.*/', ''
    $resp.Close()
}
if ($version -notmatch '^v\d') { throw "could not find the latest release (got '$version'); set `$env:AIMS_VERSION" }

$archive = "aims_$($version.TrimStart('v'))_windows_$arch.zip"
$base = "https://github.com/$repo/releases/download/$version"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "Downloading aims $version for windows/$arch"
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$archive" -OutFile (Join-Path $tmp $archive)
    Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
    $sums = Get-Content (Join-Path $tmp 'checksums.txt')
    $want = ($sums | Where-Object { $_ -match "\s$([regex]::Escape($archive))$" }) -replace '\s.*', ''
    if (-not $want) { throw "$archive is not listed in checksums.txt" }
    $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLower()
    if ($got -ne $want.Trim().ToLower()) { throw "checksum mismatch for $archive" }

    Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath $tmp -Force
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    Copy-Item (Join-Path $tmp 'aims.exe') (Join-Path $binDir 'aims.exe') -Force
} finally {
    Remove-Item -Recurse -Force $tmp
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $binDir) {
    [Environment]::SetEnvironmentVariable('Path', "$binDir;$userPath", 'User')
    Write-Host "Added $binDir to your PATH (open a new terminal)."
}
Write-Host "Installed $(& (Join-Path $binDir 'aims.exe') --version)"
Write-Host 'Next: aims init'
