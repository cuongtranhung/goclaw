# GoClaw Workspace Rules

- **Process Management**: Always kill any existing `goclaw.exe` processes before starting a new instance to prevent port conflicts or state corruption.
- **MinIO Configuration**: Use `.env.local` for environment variables when running on this host.
- **Windows Compatibility**: Use `cmd /C` or PowerShell commands for shell execution tools when running on Windows.
