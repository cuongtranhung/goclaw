# Build and install pgvector for PostgreSQL 18 using MinGW GCC

$mingwBin = "C:\ProgramData\mingw64\mingw64\bin"
$pgDir = "C:\Program Files\PostgreSQL\18"
$pgInclude = "$pgDir\include\server"
$pgLib = "$pgDir\lib"
$pgExtDir = "$pgDir\share\extension"
$srcDir = "$env:TEMP\pgvector-src\pgvector-master"

# Add MinGW to PATH
$env:PATH = "$mingwBin;$env:PATH"

Write-Host "=== GCC version ==="
& "$mingwBin\gcc.exe" --version

# Create pg_strtof stub (not exported from postgres.lib on Windows)
$stubSrc = "$srcDir\src\pg_strtof_stub.c"
$stubContent = @'
/*
 * pg_strtof stub for Windows MinGW builds.
 * PostgreSQL's pg_strtof is not exported from postgres.lib, so we provide
 * a compatible implementation using the C-locale strtof.
 */
#include <stdlib.h>
#include <locale.h>

float pg_strtof(const char *nptr, char **endptr);

float pg_strtof(const char *nptr, char **endptr)
{
    /* Use strtof directly - MinGW strtof handles "C" locale by default */
    return strtof(nptr, endptr);
}
'@
Set-Content -Path $stubSrc -Value $stubContent
Write-Host "`nCreated pg_strtof stub at $stubSrc"

Write-Host "`n=== Compiling source files ==="
$sources = Get-ChildItem "$srcDir\src" -Filter "*.c" | Select-Object -ExpandProperty FullName
$objs = @()

foreach ($src in $sources) {
    $obj = $src -replace '\.c$', '.o'
    $objs += $obj
    $name = Split-Path $src -Leaf
    Write-Host "Compiling $name..."

    $args = @(
        "-Wall", "-O2",
        "-DBUILDING_DLL",
        "-D_DLL", "-D__STDC__",
        "-I$pgInclude",
        "-I$pgInclude\port\win32_msvc",
        "-I$pgInclude\port\win32",
        "-I$pgDir\include",
        "-c", "-o", $obj, $src
    )

    $result = & "$mingwBin\gcc.exe" @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAILED: $result"
        exit 1
    }
    Write-Host "  OK"
}

Write-Host "`n=== Linking vector.dll ==="
$dllPath = "$srcDir\vector.dll"

$linkArgs = @(
    "-shared",
    "-o", $dllPath
) + $objs + @(
    "-L$pgLib",
    "$pgLib\postgres.lib"
)

$result = & "$mingwBin\gcc.exe" @linkArgs 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "Link FAILED: $result"
    exit 1
}
Write-Host "Linked successfully: $dllPath"

Write-Host "`n=== Installing to PostgreSQL ==="
Copy-Item $dllPath "$pgLib\vector.dll" -Force
Write-Host "Copied vector.dll -> $pgLib"

Copy-Item "$srcDir\vector.control" "$pgExtDir\" -Force
Write-Host "Copied vector.control -> $pgExtDir"

Get-ChildItem "$srcDir\sql" -Filter "*.sql" | ForEach-Object {
    Copy-Item $_.FullName "$pgExtDir\" -Force
    Write-Host "Copied $($_.Name) -> $pgExtDir"
}

Write-Host "`n=== Done! Testing extension ==="
$env:PGPASSWORD = "@abcd1234"
& "$pgDir\bin\psql.exe" -U postgres -d goclaw -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>&1
