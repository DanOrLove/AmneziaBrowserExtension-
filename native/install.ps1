param([switch]$Uninstall)
$ErrorActionPreference = "Stop"
$nativeDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$hostExe = Join-Path $nativeDir "AmneziaBrowserVpnHost.exe"
$hostManifest = Join-Path $nativeDir "com.amnezia.browser_vpn.json"
$hostName = "com.amnezia.browser_vpn"
$registryPaths = @(
    "HKCU:\Software\Google\Chrome\NativeMessagingHosts\$hostName",
    "HKCU:\Software\Microsoft\Edge\NativeMessagingHosts\$hostName"
)
if ($Uninstall) {
    foreach ($path in $registryPaths) { Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue }
    Remove-Item -LiteralPath $hostManifest -Force -ErrorAction SilentlyContinue
    Write-Host "Регистрация Amnezia Browser VPN удалена."
    exit 0
}
if (-not (Test-Path -LiteralPath $hostExe)) { throw "Сначала запустите build.ps1: $hostExe не найден." }
$extensionManifest = Get-Content -LiteralPath (Join-Path $nativeDir "..\extension\manifest.json") -Raw | ConvertFrom-Json
$keyBytes = [Convert]::FromBase64String($extensionManifest.key)
$hashAlgorithm = [System.Security.Cryptography.SHA256]::Create()
try { $sha = $hashAlgorithm.ComputeHash($keyBytes) } finally { $hashAlgorithm.Dispose() }
$extensionId = -join ($sha[0..15] | ForEach-Object { $n = $_; [char](97 + ($n -shr 4)); [char](97 + ($n -band 15)) })
$manifest = [ordered]@{
    name = $hostName
    description = "Local AmneziaWG SOCKS5 tunnel for Chromium"
    path = $hostExe
    type = "stdio"
    allowed_origins = @("chrome-extension://$extensionId/")
}
$manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $hostManifest -Encoding ascii
foreach ($path in $registryPaths) {
    New-Item -Path $path -Force | Out-Null
    Set-Item -Path $path -Value $hostManifest
}
Write-Host "Локальный компонент зарегистрирован для Chromium/Chrome и Edge."
Write-Host "ID расширения: $extensionId"
Write-Host "Путь Native Messaging: $hostManifest"
