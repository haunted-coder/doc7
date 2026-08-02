@echo off
setlocal

set "ROOT=%~dp0"
if "%~1"=="" (
  "%ROOT%doc7.exe" doctor
) else (
  "%ROOT%doc7.exe" doctor "%~1"
)
set "EXIT_CODE=%ERRORLEVEL%"

if not "%EXIT_CODE%"=="0" (
  echo.
  echo The dependency check failed. Install the renderer required by the input,
  echo or use a portable kit that contains tools beside doc7.exe.
)

exit /b %EXIT_CODE%
