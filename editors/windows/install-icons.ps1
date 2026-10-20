# Associe les icônes Chili aux fichiers .chl et .chlb dans l'Explorateur Windows.
# Agit uniquement sur le compte courant (HKCU), sans droits administrateur.
#
#   powershell -ExecutionPolicy Bypass -File install-icons.ps1             # installe
#   powershell -ExecutionPolicy Bypass -File install-icons.ps1 -Uninstall  # retire

param([switch]$Uninstall)

$classes = 'HKCU:\Software\Classes'
$types = @(
    @{ Ext = '.chl';  ProgId = 'Chili.Source';   Label = 'Programme Chili'; Icon = 'chili.ico' },
    @{ Ext = '.chlb'; ProgId = 'Chili.Bytecode'; Label = 'Bytecode Chili';  Icon = 'chili-bytecode.ico' }
)

foreach ($t in $types) {
    $extKey  = Join-Path $classes $t.Ext
    $progKey = Join-Path $classes $t.ProgId

    if ($Uninstall) {
        if ((Get-ItemProperty $extKey -ErrorAction SilentlyContinue).'(default)' -eq $t.ProgId) {
            Remove-Item $extKey -Recurse -Force
        }
        Remove-Item $progKey -Recurse -Force -ErrorAction SilentlyContinue
        continue
    }

    $icon = Join-Path $PSScriptRoot $t.Icon
    New-Item $extKey -Force | Out-Null
    Set-Item $extKey -Value $t.ProgId
    New-Item $progKey -Force | Out-Null
    Set-Item $progKey -Value $t.Label
    New-Item "$progKey\DefaultIcon" -Force | Out-Null
    Set-Item "$progKey\DefaultIcon" -Value "`"$icon`",0"
}

# Prévient l'Explorateur que les associations ont changé (rafraîchit les icônes).
Add-Type -Namespace Win32 -Name Shell -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("shell32.dll")]
public static extern void SHChangeNotify(int eventId, int flags, System.IntPtr item1, System.IntPtr item2);
'@
[Win32.Shell]::SHChangeNotify(0x08000000, 0, [IntPtr]::Zero, [IntPtr]::Zero)

if ($Uninstall) { 'Icônes Chili retirées.' } else { 'Icônes Chili installées pour .chl et .chlb.' }
