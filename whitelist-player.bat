@echo off
setlocal EnableExtensions EnableDelayedExpansion

cd /d "%~dp0"

if "%~1"=="" goto :usage

where go >nul 2>&1
if errorlevel 1 (
  echo Go is required to run this script.
  echo Install Go and make sure the "go" command is available in PATH.
  pause
  exit /b 1
)

if not exist ".env" (
  if not exist ".env.example" (
    echo .env.example was not found.
    pause
    exit /b 1
  )

  copy /Y ".env.example" ".env" >nul
  echo Created .env from .env.example
)

for /f "usebackq eol=# tokens=1,* delims==" %%A in (".env") do (
  if not "%%~A"=="" (
    set "%%~A=%%~B"
  )
)

set "TWITCH_LOGIN="
set "TWITCH_DISPLAY_NAME="
set "TWITCH_USER_ID="

echo %~1| findstr /R "^[0-9][0-9]*$" >nul
if not errorlevel 1 (
  if "%~2"=="" goto :usage
  set "TWITCH_USER_ID=%~1"
  set "TWITCH_LOGIN=%~2"
  set "TWITCH_DISPLAY_NAME=%~3"
) else (
  set "TWITCH_LOGIN=%~1"
  set "TWITCH_DISPLAY_NAME=%~2"
)

if not defined TWITCH_DISPLAY_NAME set "TWITCH_DISPLAY_NAME=%TWITCH_LOGIN%"

if defined TWITCH_USER_ID (
  go run ./cmd/whitelist --user-id "%TWITCH_USER_ID%" --login "%TWITCH_LOGIN%" --display-name "%TWITCH_DISPLAY_NAME%"
) else (
  go run ./cmd/whitelist --login "%TWITCH_LOGIN%" --display-name "%TWITCH_DISPLAY_NAME%"
)
set "EXIT_CODE=%ERRORLEVEL%"

if not "%EXIT_CODE%"=="0" (
  echo.
  echo Failed to whitelist the player.
  pause
)

exit /b %EXIT_CODE%

:usage
echo Usage:
echo   whitelist-player.bat ^<twitch_login^> ["Display Name"]
echo   whitelist-player.bat ^<twitch_user_id^> ^<twitch_login^> ["Display Name"]
echo Examples:
echo   whitelist-player.bat some_streamer "Some Streamer"
echo   whitelist-player.bat 123456789 some_streamer "Some Streamer"
exit /b 1
