package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"runtime"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/jungletek/Nugs-Downloader/pkg/fsutil"
	"github.com/jungletek/Nugs-Downloader/pkg/logger"
)

// Config represents the application configuration
type Config struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	Token         string `json:"token"`
	Format        int    `json:"format"`
	VideoFormat   int    `json:"videoFormat"`
	OutPath       string `json:"outPath"`
	WantRes       string
	FfmpegNameStr string
	Urls          []string
	ForceVideo    bool
	SkipVideos    bool
	SkipChapters  bool
	UseFfmpegEnvVar bool `json:"useFfmpegEnvVar"`
}

// Args represents command line arguments
type Args struct {
	Urls         []string `arg:"positional" help:"URLs to process"`
	Format       int      `arg:"-f,--format" help:"Audio format (1-5)"`
	VideoFormat  int      `arg:"-v,--video-format" help:"Video format (1-5)"`
	OutPath      string   `arg:"-o,--output" help:"Output directory"`
	ForceVideo   bool     `arg:"--force-video" help:"Force video download"`
	SkipVideos   bool     `arg:"--skip-videos" help:"Skip video downloads"`
	SkipChapters bool     `arg:"--skip-chapters" help:"Skip chapter metadata"`
}

// ParseCfg parses configuration from config.json and command line arguments
func ParseCfg() (*Config, error) {
	cfg, err := readConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	args := parseArgs()
	if args.Format != -1 {
		cfg.Format = args.Format
	}
	if args.VideoFormat != -1 {
		cfg.VideoFormat = args.VideoFormat
	}

	// Validate format ranges
	if !(cfg.Format >= MinAudioFormat && cfg.Format <= MaxAudioFormat) {
		return nil, fmt.Errorf("track format must be between %d and %d", MinAudioFormat, MaxAudioFormat)
	}
	if !(cfg.VideoFormat >= MinVideoFormat && cfg.VideoFormat <= MaxVideoFormat) {
		return nil, fmt.Errorf("video format must be between %d and %d", MinVideoFormat, MaxVideoFormat)
	}

	// Set resolution and output path
	cfg.WantRes = resolveRes[cfg.VideoFormat]
	if args.OutPath != "" {
		cfg.OutPath = args.OutPath
	}
	if cfg.OutPath == "" {
		cfg.OutPath = "Nugs downloads"
	}

	// Clean token
	if cfg.Token != "" {
		cfg.Token = strings.TrimPrefix(cfg.Token, "Bearer ")
	}

	// Set ffmpeg path based on platform
	if cfg.UseFfmpegEnvVar || runtime.GOOS == "windows" {
		cfg.FfmpegNameStr = "ffmpeg"
	} else {
		cfg.FfmpegNameStr = "./ffmpeg"
	}

	// Process URLs
	cfg.Urls, err = processUrls(args.Urls)
	if err != nil {
		logger.GetLogger().Error("Failed to process URLs", "error", err)
		return nil, err
	}

	// Set flags
	cfg.ForceVideo = args.ForceVideo
	cfg.SkipVideos = args.SkipVideos
	cfg.SkipChapters = args.SkipChapters

	return cfg, nil
}

// readConfig reads configuration from config.json
func readConfig() (*Config, error) {
	data, err := ioutil.ReadFile("config.json")
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

// parseArgs parses command line arguments
func parseArgs() *Args {
	var args Args
	arg.MustParse(&args)
	return &args
}

// processUrls processes URL arguments, handling text files
func processUrls(urls []string) ([]string, error) {
	var processed []string

	for _, url := range urls {
		if strings.HasSuffix(url, ".txt") {
			lines, err := fsutil.ReadTxtFile(url)
			if err != nil {
				return nil, err
			}
			for _, line := range lines {
				if !contains(processed, line) {
					processed = append(processed, strings.TrimSuffix(line, "/"))
				}
			}
		} else {
			if !contains(processed, url) {
				processed = append(processed, strings.TrimSuffix(url, "/"))
			}
		}
	}

	return processed, nil
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
