@echo off
rem Double-click to start the database, backend and website. See start-dev.ps1.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-dev.ps1"
echo.
pause
