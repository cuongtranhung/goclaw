# Find cl.exe
$vswhere = "C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe"
if (Test-Path $vswhere) {
    $installPath = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath 2>&1
    Write-Host "VS Install: $installPath"

    $clPaths = Get-ChildItem "$installPath\VC\Tools\MSVC" -Filter "cl.exe" -Recurse -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -like "*x64*" } |
        Select-Object -ExpandProperty FullName
    foreach ($p in $clPaths) { Write-Host "cl.exe: $p" }
} else {
    Write-Host "vswhere not found, searching..."
    Get-ChildItem "C:\Program Files (x86)\Microsoft Visual Studio" -Filter "cl.exe" -Recurse -ErrorAction SilentlyContinue | Select-Object FullName
    Get-ChildItem "C:\Program Files\Microsoft Visual Studio" -Filter "cl.exe" -Recurse -ErrorAction SilentlyContinue | Select-Object FullName
}

# Also check nmake
Write-Host "`nSearching nmake.exe..."
Get-ChildItem "C:\Program Files (x86)\Microsoft Visual Studio" -Filter "nmake.exe" -Recurse -ErrorAction SilentlyContinue | Select-Object -First 5 FullName
