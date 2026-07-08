@echo off
setlocal EnableExtensions EnableDelayedExpansion

cd /d "%~dp0"

echo Minecraft Speedrun Leaderboard Go local launcher
echo.

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

if not defined PORT set "PORT=3000"
if not defined APP_BASE_URL set "APP_BASE_URL=http://localhost:%PORT%"
if not defined UPLOAD_DIR set "UPLOAD_DIR=data/uploads"

set "UPLOAD_DIR=!UPLOAD_DIR:/=\!"

if not exist "!UPLOAD_DIR!" (
  mkdir "!UPLOAD_DIR!" >nul 2>&1
)

if /I "%~1"=="--check" (
  echo Environment file: .env
  echo App URL: !APP_BASE_URL!
  echo Database URL: !DATABASE_URL!
  echo Dev mock auth: !ALLOW_DEV_MOCK_AUTH!
  echo Upload dir: !UPLOAD_DIR!
  exit /b 0
)

echo Environment file: .env
echo App URL: !APP_BASE_URL!
echo Dev mock auth: !ALLOW_DEV_MOCK_AUTH!
echo Upload dir: !UPLOAD_DIR!
echo.
echo Make sure PostgreSQL is running before starting the server.
echo Press Ctrl+C to stop the Go service.
echo.

go run ./cmd/server
set "EXIT_CODE=%ERRORLEVEL%"

if not "%EXIT_CODE%"=="0" (
  echo.
  echo The Go service exited with code %EXIT_CODE%.
  echo Check whether PostgreSQL is running and whether your .env values are correct.
  pause
)

exit /b %EXIT_CODE%
