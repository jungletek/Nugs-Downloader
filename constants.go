package main

const (
	// Filename length limits
	MaxFolderNameLen     = 120
	MaxVideoFilenameLen  = 110
	MaxTrackFilenameLen  = 255

	// File permissions (cross-platform)
	DefaultFilePerms = 0644
	DefaultDirPerms  = 0755

	// API and network settings
	RequestTimeout     = 30 // seconds
	PaginationLimit    = 100
	MaxRetries         = 3

	// Download settings
	BufferSize = 32 * 1024 // 32KB buffer for I/O operations

	// Progress reporting
	ProgressReportInterval = 1 // second between progress updates
)
