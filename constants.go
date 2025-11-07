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

	// Quality format ranges
	MinAudioFormat  = 1
	MaxAudioFormat  = 5
	MinVideoFormat  = 1
	MaxVideoFormat  = 5

	// Crypto buffer sizes
	AESKeySize = 16 // bytes for AES key

	// Stream metadata retry attempts
	StreamMetaAttempts = 4

	// Regex pattern count
	URLRegexCount = 11

	// Quality format mappings
	FormatALAC     = 1
	FormatFLAC     = 2
	FormatMQA      = 3
	Format360RA    = 4
	FormatAAC      = 5

	// Video resolution mappings
	Res480p  = "480"
	Res720p  = "720"
	Res1080p = "1080"
	Res1440p = "1440"
	Res2160p = "2160" // 4K

	// Fallback mappings
	TrackFallback1 = 2
	TrackFallback2 = 5
	TrackFallback3 = 2
	TrackFallback4 = 3

	ResFallback720  = "480"
	ResFallback1080 = "720"
	ResFallback1440 = "1080"
)
