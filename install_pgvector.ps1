# Install pgvector for PostgreSQL 18 on Windows

$pgDir = "C:\Program Files\PostgreSQL\18"
$pgBin = "$pgDir\bin"
$pgLib = "$pgDir\lib"
$pgShare = "$pgDir\share\extension"

# Check pg_config
Write-Host "PostgreSQL dir: $pgDir"
Write-Host "Checking available compilers..."

$cl = Get-Command cl.exe -ErrorAction SilentlyContinue
$nmake = Get-Command nmake.exe -ErrorAction SilentlyContinue

if ($cl) { Write-Host "MSVC cl.exe: $($cl.Source)" }
else { Write-Host "MSVC cl.exe: NOT FOUND" }

if ($nmake) { Write-Host "nmake.exe: $($nmake.Source)" }
else { Write-Host "nmake.exe: NOT FOUND" }

# Download pgvector source
Write-Host "`nDownloading pgvector source..."
$pgvectorUrl = "https://github.com/pgvector/pgvector/archive/refs/heads/master.zip"
$zipPath = "$env:TEMP\pgvector.zip"
$extractPath = "$env:TEMP\pgvector-src"

try {
    Invoke-WebRequest -Uri $pgvectorUrl -OutFile $zipPath -UseBasicParsing
    Write-Host "Downloaded to $zipPath"

    if (Test-Path $extractPath) { Remove-Item $extractPath -Recurse -Force }
    Expand-Archive -Path $zipPath -DestinationPath $extractPath
    Write-Host "Extracted to $extractPath"

    $srcDir = Get-ChildItem $extractPath | Select-Object -First 1
    Write-Host "Source directory: $($srcDir.FullName)"
    Write-Host "Contents:"
    Get-ChildItem $srcDir.FullName | Select-Object Name
} catch {
    Write-Host "Error: $_"
}
