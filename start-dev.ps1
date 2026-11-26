# Starts everything needed to run the platform on this computer:
# the PostgreSQL container, the Go API (port 8080) and the website (port 3000).
# Each server opens in its own window; close a window to stop that server.
# Anything already running is left alone. Run it with start-dev.cmd.

$root = $PSScriptRoot

function Test-Port($port) {
    [bool](Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue)
}

function Wait-Port($port, $seconds) {
    for ($i = 0; $i -lt $seconds; $i++) {
        if (Test-Port $port) { return $true }
        Start-Sleep -Seconds 1
    }
    return $false
}

Write-Host ""
Write-Host "Digital Healthcare Platform - starting the development servers" -ForegroundColor Cyan
Write-Host ""

# 1. Database (Docker)
# Docker prints progress on stderr; go through cmd so only the exit code counts.
cmd /c "docker info >nul 2>&1"
if ($LASTEXITCODE -ne 0) {
    Write-Host "Docker is not running. Open Docker Desktop, wait until it says 'running', then run this again." -ForegroundColor Red
    exit 1
}
Push-Location $root
# Keep Docker's message: it says why the database could not start (e.g. a port in use).
$dbOutput = cmd /c "docker compose up -d postgres 2>&1"
Pop-Location
if (Wait-Port 45432 60) {
    Write-Host "[ok] Database        127.0.0.1:45432" -ForegroundColor Green
} else {
    Write-Host "The database did not start. Docker said:" -ForegroundColor Red
    $dbOutput | Select-Object -Last 3 | ForEach-Object { Write-Host "  $_" }
    exit 1
}

# 2. Backend (Go API)
if (Test-Port 8080) {
    Write-Host "[ok] Backend API     already running on http://localhost:8080" -ForegroundColor Green
} else {
    Start-Process powershell -WorkingDirectory $root -ArgumentList @(
        "-NoExit", "-Command",
        "`$Host.UI.RawUI.WindowTitle = 'Backend API (port 8080) - close to stop'; go run ./cmd/api"
    )
    Write-Host "     Backend API     starting (first start compiles, ~30 s)..."
    if (Wait-Port 8080 180) {
        Write-Host "[ok] Backend API     http://localhost:8080  (docs: /docs)" -ForegroundColor Green
    } else {
        Write-Host "The backend did not start. Look at its window for the error." -ForegroundColor Red
    }
}

# 3. Website (Next.js)
if (Test-Port 3000) {
    Write-Host "[ok] Website         already running on http://localhost:3000" -ForegroundColor Green
} else {
    $frontend = Join-Path $root "frontend"
    if (-not (Test-Path (Join-Path $frontend "node_modules"))) {
        Write-Host "     Website         installing packages (first time only)..."
        Push-Location $frontend
        npm install --no-fund --no-audit
        Pop-Location
    }
    Start-Process powershell -WorkingDirectory $frontend -ArgumentList @(
        "-NoExit", "-Command",
        "`$Host.UI.RawUI.WindowTitle = 'Website (port 3000) - close to stop'; npm run dev"
    )
    Write-Host "     Website         starting..."
    if (Wait-Port 3000 120) {
        Write-Host "[ok] Website         http://localhost:3000" -ForegroundColor Green
    } else {
        Write-Host "The website did not start. Look at its window for the error." -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "Ready. Opening http://localhost:3000 ..." -ForegroundColor Cyan
Start-Process "http://localhost:3000"
