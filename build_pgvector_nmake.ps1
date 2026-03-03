# Build pgvector using official Makefile.win + NMAKE + MSVC

$vsBase = "C:\Program Files\Microsoft Visual Studio\18\Community"
$msvcVer = "14.50.35717"
$clDir = "$vsBase\VC\Tools\MSVC\$msvcVer\bin\Hostx64\x64"
$sdkVer = "10.0.26100.0"
$pgRoot = "C:\Program Files\PostgreSQL\18"
$srcDir = "$env:TEMP\pgvector-src\pgvector-master"

# Set up full MSVC environment (same as vcvars64.bat)
$env:PATH = "$clDir;$env:PATH"
$env:INCLUDE = "$vsBase\VC\Tools\MSVC\$msvcVer\include;" +
               "C:\Program Files (x86)\Windows Kits\10\Include\$sdkVer\ucrt;" +
               "C:\Program Files (x86)\Windows Kits\10\Include\$sdkVer\um;" +
               "C:\Program Files (x86)\Windows Kits\10\Include\$sdkVer\shared"
$env:LIB = "$vsBase\VC\Tools\MSVC\$msvcVer\lib\x64;" +
           "C:\Program Files (x86)\Windows Kits\10\Lib\$sdkVer\ucrt\x64;" +
           "C:\Program Files (x86)\Windows Kits\10\Lib\$sdkVer\um\x64"

Write-Host "=== Building pgvector with NMAKE + MSVC ==="
Write-Host "Source: $srcDir"
Write-Host "PGROOT: $pgRoot"

Set-Location $srcDir

$result = & "$clDir\nmake.exe" /f "Makefile.win" "PGROOT=$pgRoot" 2>&1
$result | ForEach-Object { Write-Host $_ }

if ($LASTEXITCODE -ne 0) {
    Write-Host "`nBuild FAILED (exit $LASTEXITCODE)"
    exit 1
}

Write-Host "`n=== Installing ==="
$result = & "$clDir\nmake.exe" /f "Makefile.win" "PGROOT=$pgRoot" install 2>&1
$result | ForEach-Object { Write-Host $_ }

if ($LASTEXITCODE -ne 0) {
    Write-Host "`nInstall FAILED"
    exit 1
}

Write-Host "`n=== Testing extension ==="
$env:PGPASSWORD = "@abcd1234"
$result = & "$pgRoot\bin\psql.exe" -U postgres -d goclaw -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>&1
Write-Host $result
