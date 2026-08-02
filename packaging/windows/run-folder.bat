@echo off
setlocal

if "%~1"=="" (
  echo Usage: drag a directory containing supported documents onto this file.
  pause
  exit /b 1
)

set "ROOT=%~dp0"
set "INPUT=%~f1"
set "OUTDIR=%~f1-doc7"

"%ROOT%doc7.exe" "%INPUT%" -o "%OUTDIR%"
set "EXIT_CODE=%ERRORLEVEL%"

if "%EXIT_CODE%"=="0" echo Output: %OUTDIR%
pause
exit /b %EXIT_CODE%
