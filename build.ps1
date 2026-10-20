# Construit tout Chili : firmware ESP32 puis chili.exe (qui embarque le firmware).
#
#   powershell -ExecutionPolicy Bypass -File build.ps1            # tout
#   powershell -ExecutionPolicy Bypass -File build.ps1 -SkipFirmware
#   powershell -ExecutionPolicy Bypass -File build.ps1 -Installer     # + dist\ChiliSetup-x.y.z.exe
#
# Outils de développement nécessaires : Go, et PlatformIO pour le firmware
# (python -m pip install platformio), Inno Setup pour l'installateur
# (winget install JRSoftware.InnoSetup --scope user). Les utilisateurs n'en ont pas besoin.

param([switch]$SkipFirmware, [switch]$Installer)
$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$fwOut = Join-Path $root 'compiler\firmware\esp32'

if (-not $SkipFirmware) {
    Write-Host '== Firmware ESP32'
    python -m platformio run -d (Join-Path $root 'runtimes\esp32')
    if ($LASTEXITCODE -ne 0) { throw 'échec de la compilation du firmware' }

    $build = Join-Path $root 'runtimes\esp32\.pio\build\esp32dev'
    $bootApp0 = Join-Path $env:USERPROFILE '.platformio\packages\framework-arduinoespressif32\tools\partitions\boot_app0.bin'
    New-Item -ItemType Directory -Force $fwOut | Out-Null
    Copy-Item (Join-Path $build 'bootloader.bin'), (Join-Path $build 'partitions.bin'), (Join-Path $build 'firmware.bin'), $bootApp0 $fwOut

    # Version du firmware, lue dans platformio.ini (-DCHILI_FW_VERSION=\"x.y.z\").
    $ini = Get-Content (Join-Path $root 'runtimes\esp32\platformio.ini') -Raw
    if ($ini -notmatch 'CHILI_FW_VERSION=\\"([^\\"]+)\\"') { throw 'CHILI_FW_VERSION introuvable dans platformio.ini' }
    [IO.File]::WriteAllText((Join-Path $fwOut 'version.txt'), $Matches[1])
}

Write-Host '== chili.exe'
Push-Location (Join-Path $root 'compiler')
try {
    go build -o (Join-Path $root 'chili.exe') ./cmd/chili
    if ($LASTEXITCODE -ne 0) { throw 'échec de la compilation de chili.exe' }
} finally {
    Pop-Location
}
Write-Host "OK : $(Join-Path $root 'chili.exe')"

if ($Installer) {
    Write-Host '== Installateur'
    $iscc = Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe'
    if (-not (Test-Path $iscc)) { $iscc = 'C:\Program Files (x86)\Inno Setup 6\ISCC.exe' }
    $version = (Select-String -Path (Join-Path $root 'compiler\cmd\chili\main.go') -Pattern 'const version = "([^"]+)"').Matches[0].Groups[1].Value
    & $iscc /Q "/DAppVersion=$version" (Join-Path $root 'installer\chili.iss')
    if ($LASTEXITCODE -ne 0) { throw "échec de la compilation de l'installateur" }
    Write-Host "OK : $(Join-Path $root "dist\ChiliSetup-$version.exe")"
}
