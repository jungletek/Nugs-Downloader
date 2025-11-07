package main

import (
	"runtime"
	"strings"
)

// OS represents the operating system type
type OS int

const (
	Windows OS = iota
	Unix
	Unknown
)

// GetOS returns the current operating system
func GetOS() OS {
	switch strings.ToLower(runtime.GOOS) {
	case "windows":
		return Windows
	case "linux", "darwin", "freebsd", "openbsd", "netbsd":
		return Unix
	default:
		return Unknown
	}
}

// IsWindows returns true if running on Windows
func IsWindows() bool {
	return GetOS() == Windows
}

// IsUnix returns true if running on Unix-like system
func IsUnix() bool {
	return GetOS() == Unix
}
