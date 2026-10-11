@echo off
setlocal

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1"
set "INSTALL_RESULT=%ERRORLEVEL%"

if not "%INSTALL_RESULT%"=="0" (
    echo.
    echo Установка завершилась с ошибкой. Скопируйте сообщение выше.
)

echo.
pause
exit /b %INSTALL_RESULT%
