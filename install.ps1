$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

try {
    if (-not [Environment]::Is64BitOperatingSystem) {
        throw "Эта версия установщика предназначена для Windows x64."
    }

    $go = Get-Command go -ErrorAction SilentlyContinue
    if (-not $go) {
        throw "Go 1.25 или новее не найден. Установите Go с https://go.dev/dl/ и снова запустите install.bat."
    }

    Write-Host "Собираю локальный компонент. При первой сборке Go скачает зависимости; требуется интернет."
    & (Join-Path $projectRoot "native\build.ps1")

    Write-Host "Регистрирую Native Messaging host для текущего пользователя. Права администратора не нужны."
    & (Join-Path $projectRoot "native\install.ps1")

    $chromePaths = @(
        "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
        "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
        "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe"
    )
    $edgePaths = @(
        "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
        "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe",
        "$env:LOCALAPPDATA\Microsoft\Edge\Application\msedge.exe"
    )
    $browser = $chromePaths | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    if (-not $browser) {
        $browser = $edgePaths | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    }

    if ($browser) {
        Start-Process -FilePath $browser -ArgumentList "chrome://extensions"
    } else {
        Write-Host "Не удалось найти Chrome или Edge. Откройте в браузере chrome://extensions или edge://extensions."
    }

    Write-Host ""
    Write-Host "Готово. Остался ручной шаг: включите Режим разработчика, нажмите 'Загрузить распакованное расширение' и выберите папку extension в папке проекта."
    Write-Host "Не перемещайте и не удаляйте папку проекта после установки."
    exit 0
}
catch {
    Write-Host "Установка не завершена: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
