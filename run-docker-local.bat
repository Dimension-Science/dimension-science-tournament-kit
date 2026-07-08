@echo off
setlocal

cd /d "%~dp0"

echo Minecraft Speedrun Leaderboard Docker local launcher
echo.

where docker >nul 2>&1
if errorlevel 1 (
  echo Docker Desktop is required to run this script.
  echo Install Docker Desktop and make sure the "docker" command is available.
  pause
  exit /b 1
)

docker compose version >nul 2>&1
if errorlevel 1 (
  echo Docker Compose is not available.
  echo Update Docker Desktop so "docker compose" works from the terminal.
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

if not exist "data\uploads" (
  mkdir "data\uploads" >nul 2>&1
)

set "DATABASE_URL=postgres://postgres:postgres@postgres:5432/speedrun?sslmode=disable"

echo Starting local Docker stack...
echo App URL: http://localhost:3000
echo Healthcheck: http://localhost:3000/health
echo.
echo Press Ctrl+C to stop the containers.
echo.

docker compose up --build
set "EXIT_CODE=%ERRORLEVEL%"

if not "%EXIT_CODE%"=="0" (
  echo.
  echo Startup failed. Check the Docker output above.
  pause
)

exit /b %EXIT_CODE%
