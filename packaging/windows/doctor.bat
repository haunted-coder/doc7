@echo off
setlocal

set "ROOT=%~dp0"
"%ROOT%doc7.exe" doctor --check-model
set "EXIT_CODE=%ERRORLEVEL%"

if not "%EXIT_CODE%"=="0" (
  echo.
  echo Configure the model with doc7.exe setup config, then run this check again.
)

pause
exit /b %EXIT_CODE%
