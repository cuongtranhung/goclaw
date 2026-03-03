# run_minio_tests.ps1
$env_file = Get-Content .env.local
foreach ($line in $env_file) {
    if ($line -match 'export\s+([^=]+)=(.*)') {
        $name = $Matches[1].Trim()
        $value = $Matches[2].Trim()
        # Remove quotes if present
        $value = $value.Trim('"').Trim("'")
        # Mask secrets in output
        if ($name -like "*KEY*" -or $name -like "*TOKEN*") {
            Write-Host "Setting $name=********"
        } else {
            Write-Host "Setting $name=$value"
        }
        Set-Item -Path "Env:$name" -Value $value
    }
}

$GO_BIN = "C:\Go\bin\go.exe"
if (-not (Test-Path $GO_BIN)) {
    $GO_BIN = "go" # fallback to path
}

Write-Host "`nRunning Storage Tests..."
& $GO_BIN test -v ./internal/storage/minio/...

Write-Host "`nRunning Tools Tests (filtering for Minio)..."
& $GO_BIN test -v ./internal/tools/ -run Minio
