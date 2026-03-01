//go:build windows

package tools

import (
	"fmt"
	"log/slog"
	"os"
	"syscall"
)

// hasMutableSymlinkParent on Windows: symlinks require admin privileges and are
// uncommon, so TOCTOU symlink rebind attacks via writable parent are not applicable.
func hasMutableSymlinkParent(path string) bool {
	return false
}

// checkHardlink rejects regular files with nlink > 1 (hardlink attack prevention).
// Directories naturally have nlink > 1 and are exempt.
func checkHardlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return nil // non-existent files are OK — will fail at read/write
	}
	if info.IsDir() {
		return nil
	}
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil
	}
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(handle)
	var fi syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &fi); err != nil {
		return nil
	}
	if fi.NumberOfLinks > 1 {
		slog.Warn("security.hardlink_rejected", "path", path, "nlink", fi.NumberOfLinks)
		return fmt.Errorf("access denied: hardlinked file not allowed")
	}
	return nil
}
