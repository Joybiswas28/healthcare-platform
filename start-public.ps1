# Shares this computer's copy of the website on a temporary public https link
# (a Cloudflare quick tunnel), so others can open it from their phone or PC.
#
# - The website runs as a production build on port 3200: the login page does
#   NOT list the demo accounts. Your development site on port 3000 is untouched.
# - The link works only while this computer and both windows stay open, and it
#   changes every time. Close the two windows to stop sharing.
# - Anyone with the link can reach the site. Give the link and logins only to
#   people you trust; the demo passwords are in the code on GitHub.
#
# Needs start-dev.cmd to be running (database and backend). Run with start-public.cmd.

$root = $PSScriptRoot
$frontend = Join-Path $root "frontend"
$port = 3200

function Test-Port($p) {
    [bool](Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue)
}

Write-Host ""
Write-Host "Digital Healthcare Platform - public link" -ForegroundColor Cyan
Write-Host ""

$cloudflared = (Get-Command cloudflared -ErrorAction SilentlyContinue).Source
if (-not $cloudflared) {
    $cloudflared = @("${env:ProgramFiles(x86)}\cloudflared\cloudflared.exe", "$env:ProgramFiles\cloudflared\cloudflared.exe") |
        Where-Object { Test-Path $_ } | Select-Object -First 1
}
if (-not $cloudflared) {
    Write-Host "cloudflared is not installed. Install it once with:" -ForegroundColor Red
    Write-Host "  winget install --id Cloudflare.cloudflared -e"
    exit 1
}

if (-not (Test-Port 8080)) {
    Write-Host "The backend is not running. Start start-dev.cmd first, then run this again." -ForegroundColor Red
    exit 1
}

# 1. Website, production build
if (Test-Port $port) {
    Write-Host "[ok] Website         already running on port $port" -ForegroundColor Green
} else {
    Write-Host "     Website         building the production version (1-2 minutes)..."
    Push-Location $frontend
    $env:NEXT_TELEMETRY_DISABLED = "1"
    npm run build *> (Join-Path $env:TEMP "healthcare-public-build.log")
    $built = $LASTEXITCODE -eq 0
    Pop-Location
    if (-not $built) {
        Write-Host "The build failed. Details: $env:TEMP\healthcare-public-build.log" -ForegroundColor Red
        exit 1
    }
    Start-Process powershell -WorkingDirectory $frontend -ArgumentList @(
        "-NoExit", "-Command",
        "`$Host.UI.RawUI.WindowTitle = 'Public website (port $port) - close to stop'; npx next start -p $port"
    )
    for ($i = 0; $i -lt 60 -and -not (Test-Port $port); $i++) { Start-Sleep -Seconds 1 }
    if (-not (Test-Port $port)) {
        Write-Host "The website did not start. Look at its window for the error." -ForegroundColor Red
        exit 1
    }
    Write-Host "[ok] Website         port $port (production)" -ForegroundColor Green
}

# 2. Tunnel
$log = Join-Path $env:TEMP "healthcare-tunnel.log"
Remove-Item $log -ErrorAction SilentlyContinue
Start-Process powershell -ArgumentList @(
    "-NoExit", "-Command",
    "`$Host.UI.RawUI.WindowTitle = 'Public link - close to stop sharing'; & '$cloudflared' tunnel --no-autoupdate --url http://localhost:$port --logfile '$log'"
)
Write-Host "     Public link     connecting..."
$url = $null
for ($i = 0; $i -lt 60 -and -not $url; $i++) {
    Start-Sleep -Seconds 1
    if (Test-Path $log) {
        $m = Select-String -Path $log -Pattern "https://[a-z0-9-]+\.trycloudflare\.com" | Select-Object -First 1
        if ($m) { $url = $m.Matches[0].Value }
    }
}
if (-not $url) {
    Write-Host "No link yet. Look at the 'Public link' window for the address or an error." -ForegroundColor Red
    exit 1
}

Set-Clipboard -Value $url
Write-Host ""
Write-Host "Public link (copied to the clipboard):" -ForegroundColor Cyan
Write-Host "  $url" -ForegroundColor Yellow
Write-Host ""
Write-Host "It works while this computer and the two new windows stay open."
Write-Host "Close the 'Public link' window to stop sharing."
