# Build pgvector for PostgreSQL 18 using MSVC (proper ABI compatibility)

$vsBase = "C:\Program Files\Microsoft Visual Studio\18\Community"
$msvcVer = "14.50.35717"
$clDir = "$vsBase\VC\Tools\MSVC\$msvcVer\bin\Hostx64\x64"
$pgDir = "C:\Program Files\PostgreSQL\18"
$pgInclude = "$pgDir\include\server"
$pgLib = "$pgDir\lib"
$pgExtDir = "$pgDir\share\extension"
$srcDir = "$env:TEMP\pgvector-src\pgvector-master\src"
$outDir = "$env:TEMP\pgvector-src\pgvector-master"

# Set up MSVC environment
$env:PATH = "$clDir;$env:PATH"
$sdkVer = "10.0.26100.0"
$env:INCLUDE = "$vsBase\VC\Tools\MSVC\$msvcVer\include;" +
               "$vsBase\VC\Tools\MSVC\$msvcVer\atlmfc\include;" +
               "C:\Program Files (x86)\Windows Kits\10\Include\$sdkVer\ucrt;" +
               "C:\Program Files (x86)\Windows Kits\10\Include\$sdkVer\um;" +
               "C:\Program Files (x86)\Windows Kits\10\Include\$sdkVer\shared"
$env:LIB = "$vsBase\VC\Tools\MSVC\$msvcVer\lib\x64;" +
           "C:\Program Files (x86)\Windows Kits\10\Lib\$sdkVer\ucrt\x64;" +
           "C:\Program Files (x86)\Windows Kits\10\Lib\$sdkVer\um\x64"

Write-Host "=== MSVC version ==="
& "$clDir\cl.exe" 2>&1 | Select-Object -First 2

# Compile each .c file
Write-Host "`n=== Compiling source files ==="
$sources = Get-ChildItem $srcDir -Filter "*.c" | Where-Object { $_.Name -ne "pg_strtof_stub.c" }
$objs = @()

foreach ($src in $sources) {
    $obj = Join-Path $outDir ($src.BaseName + ".obj")
    $objs += $obj
    Write-Host "Compiling $($src.Name)..."

    $args = @(
        "/c", "/O2", "/W3",
        "/MD",                       # Use DLL runtime (matches PostgreSQL)
        "/DBUILDING_DLL",
        "/DWIN32", "/D_WINDOWS",
        "/I$pgInclude",
        "/I$pgInclude\port\win32_msvc",
        "/I$pgInclude\port\win32",
        "/I$pgDir\include",
        "/Fo$obj",
        $src.FullName
    )

    $result = & "$clDir\cl.exe" @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAILED: $result"
        exit 1
    }
    Write-Host "  OK"
}

Write-Host "`n=== Linking vector.dll ==="
$dllPath = "$outDir\vector.dll"
$defPath = "$outDir\vector.def"

# Link DLL
$linkArgs = @(
    "/DLL",
    "/OUT:$dllPath",
    "/IMPLIB:$outDir\vector.lib"
) + $objs + @(
    "$pgLib\postgres.lib"
)

$linkExe = "$clDir\..\..\..\..\..\..\Common7\IDE\CommonExtensions\Microsoft\CMake\cmake\bin"
# Use the linker from MSVC
$linkExe = "$clDir\link.exe"
Write-Host "Linker: $linkExe"

$result = & $linkExe @linkArgs 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "Link FAILED:"
    $result | ForEach-Object { Write-Host "  $_" }
    exit 1
}
Write-Host "Linked: $dllPath"

Write-Host "`n=== Installing ==="
Copy-Item $dllPath "$pgLib\vector.dll" -Force
Write-Host "Copied vector.dll -> $pgLib"

Copy-Item "$outDir\vector.control" "$pgExtDir\" -Force

Get-ChildItem "$outDir\sql" -Filter "*.sql" | ForEach-Object {
    Copy-Item $_.FullName "$pgExtDir\" -Force
}
# Ensure version-tagged init script exists
if (-not (Test-Path "$pgExtDir\vector--0.8.2.sql")) {
    Copy-Item "$pgExtDir\vector.sql" "$pgExtDir\vector--0.8.2.sql" -Force
}

Write-Host "`n=== Testing extension ==="
$env:PGPASSWORD = "@abcd1234"
$result = & "$pgDir\bin\psql.exe" -U postgres -d goclaw -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>&1
Write-Host $result
if ($LASTEXITCODE -eq 0) {
    Write-Host "`nSUCCESS! pgvector installed correctly."
} else {
    Write-Host "`nFailed to create extension."
}
