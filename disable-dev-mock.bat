@echo off
setlocal

cd /d "%~dp0"

if not exist ".env" (
  if not exist ".env.example" (
    echo .env.example was not found.
    pause
    exit /b 1
  )

  copy /Y ".env.example" ".env" >nul
  echo Created .env from .env.example
)

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$path = '.env';" ^
  "$key = 'ALLOW_DEV_MOCK_AUTH';" ^
  "$value = 'false';" ^
  "$lines = Get-Content $path -ErrorAction Stop;" ^
  "if ($lines -match ('^' + [regex]::Escape($key) + '=')) {" ^
  "  $lines = $lines | ForEach-Object { if ($_ -match ('^' + [regex]::Escape($key) + '=')) { $key + '=' + $value } else { $_ } };" ^
  "} else {" ^
  "  $lines += $key + '=' + $value;" ^
  "}" ^
  "Set-Content -Path $path -Value $lines"

if errorlevel 1 (
  echo Failed to update .env
  pause
  exit /b 1
)

echo Dev mock authorization is DISABLED.
echo Restart the server for the change to take effect.
exit /b 0
