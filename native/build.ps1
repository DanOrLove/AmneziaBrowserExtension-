$ErrorActionPreference = "Stop"
$nativeDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$goCmd = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCmd) {
    throw "Go 1.25+ не найден. Установите Go с https://go.dev/dl/ и повторите сборку."
}
$version = (& go version) -join " "
if ($version -notmatch 'go1\.(2[5-9]|[3-9][0-9])') {
    throw "Нужен Go 1.25 или новее. Найдено: $version"
}
Push-Location $nativeDir
try {
    & go mod tidy
    if ($LASTEXITCODE -ne 0) { throw "Не удалось загрузить зависимости AmneziaWG." }
    & go build -trimpath -ldflags "-s -w" -o (Join-Path $nativeDir "AmneziaBrowserVpnHost.exe") .
    if ($LASTEXITCODE -ne 0) { throw "Сборка локального компонента завершилась ошибкой." }
} finally {
    Pop-Location
}
Write-Host "Готово: $nativeDir\AmneziaBrowserVpnHost.exe"
