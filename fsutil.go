package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Cross-platform file permission constants
const (
	// Windows ignores execute permissions, so we use different values
	DefaultFilePermsWindows = 0666
	DefaultDirPermsWindows  = 0777
	DefaultFilePermsUnix    = 0644
	DefaultDirPermsUnix     = 0755
)

// GetFileMode returns appropriate file permissions for the current platform
func GetFileMode() os.FileMode {
	if runtime.GOOS == "windows" {
		return DefaultFilePermsWindows
	}
	return DefaultFilePermsUnix
}

// GetDirMode returns appropriate directory permissions for the current platform
func GetDirMode() os.FileMode {
	if runtime.GOOS == "windows" {
		return DefaultDirPermsWindows
	}
	return DefaultDirPermsUnix
}

// PathsEqual performs case-insensitive path comparison on Windows
func PathsEqual(path1, path2 string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(path1, path2)
	}
	return path1 == path2
}

// SafeJoin performs path joining with normalization
func SafeJoin(elem ...string) string {
	path := filepath.Join(elem...)
	return filepath.Clean(path)
}

// MakeDirs creates directories with cross-platform permissions
func MakeDirs(path string) error {
	return os.MkdirAll(path, GetDirMode())
}

// OpenFile opens a file with cross-platform permissions
func OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	if perm == 0 {
		perm = GetFileMode()
	}
	return os.OpenFile(name, flag, perm)
}

// ReadFile opens a file for reading with appropriate permissions
func ReadFile(name string) (*os.File, error) {
	return OpenFile(name, os.O_RDONLY, 0)
}

// WriteFile opens a file for writing with appropriate permissions
func WriteFile(name string) (*os.File, error) {
	return OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0)
}

// AppendFile opens a file for appending with appropriate permissions
func AppendFile(name string) (*os.File, error) {
	return OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0)
}
