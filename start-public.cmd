@echo off
rem Double-click to share the website on a temporary public link. See start-public.ps1.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-public.ps1"
echo.
pause
