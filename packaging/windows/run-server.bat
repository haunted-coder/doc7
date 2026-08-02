@echo off
setlocal
cd /d "%~dp0"

echo Starting doc7 HTTP service at http://127.0.0.1:8787
echo Press Ctrl+C to stop.
echo.

"%~dp0doc7.exe" serve --addr "127.0.0.1:8787" --data-dir "%~dp0doc7-server"
if errorlevel 1 (
  echo.
  echo doc7 server stopped with an error.
  pause
)
