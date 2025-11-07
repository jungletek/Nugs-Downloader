package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// TestSuite for config package
type ConfigTestSuite struct {
	suite.Suite
	tempDir    string
	originalArgs []string
}

// SetupTest creates a temporary directory and saves original command line args
func (suite *ConfigTestSuite) SetupTest() {
	tempDir, err := os.MkdirTemp("", "config_test_*")
	assert.NoError(suite.T(), err)
	suite.tempDir = tempDir

	// Save original command line args
	suite.originalArgs = os.Args
}

// TearDownTest cleans up temporary directory and restores command line args
func (suite *ConfigTestSuite) TearDownTest() {
	if suite.tempDir != "" {
		os.RemoveAll(suite.tempDir)
	}
	// Restore original command line args
	os.Args = suite.originalArgs
}

// TestParseCfg_ValidConfig tests parsing valid configuration
func (suite *ConfigTestSuite) TestParseCfg_ValidConfig() {
	// Create a valid config.json
	configData := Config{
		Email:       "test@example.com",
		Password:    "testpass",
		Token:       "Bearer test-token",
		Format:      2,
		VideoFormat: 3,
		OutPath:     "test-output",
	}
	suite.createConfigFile(configData)

	// Set command line args
	os.Args = []string{"program", "https://example.com/album/123"}

	cfg, err := ParseCfg()

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), cfg)
	assert.Equal(suite.T(), "test@example.com", cfg.Email)
	assert.Equal(suite.T(), "testpass", cfg.Password)
	assert.Equal(suite.T(), "test-token", cfg.Token) // Bearer prefix should be removed
	assert.Equal(suite.T(), 2, cfg.Format)
	assert.Equal(suite.T(), 3, cfg.VideoFormat)
	assert.Equal(suite.T(), "1080", cfg.WantRes)
	assert.Equal(suite.T(), "test-output", cfg.OutPath)
	assert.Contains(suite.T(), cfg.Urls, "https://example.com/album/123")
}

// TestParseCfg_CommandLineOverrides tests command line argument overrides
func (suite *ConfigTestSuite) TestParseCfg_CommandLineOverrides() {
	// Create config with default values
	configData := Config{
		Format:      1,
		VideoFormat: 1,
		OutPath:     "default-output",
	}
	suite.createConfigFile(configData)

	// Set command line args with overrides
	os.Args = []string{
		"program",
		"-f", "3",
		"-v", "4",
		"-o", "cli-output",
		"--force-video",
		"--skip-videos",
		"https://example.com/test",
	}

	cfg, err := ParseCfg()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, cfg.Format)      // Overridden from CLI
	assert.Equal(suite.T(), 4, cfg.VideoFormat) // Overridden from CLI
	assert.Equal(suite.T(), "1440", cfg.WantRes)
	assert.Equal(suite.T(), "cli-output", cfg.OutPath) // Overridden from CLI
	assert.True(suite.T(), cfg.ForceVideo)
	assert.True(suite.T(), cfg.SkipVideos)
}

// TestParseCfg_InvalidFormat tests invalid format ranges
func (suite *ConfigTestSuite) TestParseCfg_InvalidFormat() {
	// Test invalid audio format
	configData := Config{
		Format:      10, // Invalid
		VideoFormat: 3,
	}
	suite.createConfigFile(configData)

	os.Args = []string{"program"}

	_, err := ParseCfg()
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "track format must be between 1 and 5")
}

// TestParseCfg_InvalidVideoFormat tests invalid video format ranges
func (suite *ConfigTestSuite) TestParseCfg_InvalidVideoFormat() {
	// Test invalid video format
	configData := Config{
		Format:      2,
		VideoFormat: 10, // Invalid
	}
	suite.createConfigFile(configData)

	os.Args = []string{"program"}

	_, err := ParseCfg()
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "video format must be between 1 and 5")
}

// TestParseCfg_DefaultOutputPath tests default output path when not specified
func (suite *ConfigTestSuite) TestParseCfg_DefaultOutputPath() {
	configData := Config{
		Format:      2,
		VideoFormat: 3,
		OutPath:     "", // Empty
	}
	suite.createConfigFile(configData)

	os.Args = []string{"program", "https://example.com/test"}

	cfg, err := ParseCfg()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Nugs downloads", cfg.OutPath)
}

// TestParseCfg_FfmpegPath_Windows tests ffmpeg path on Windows
func (suite *ConfigTestSuite) TestParseCfg_FfmpegPath_Windows() {
	if runtime.GOOS != "windows" {
		suite.T().Skip("Test only runs on Windows")
	}

	configData := Config{
		Format:      2,
		VideoFormat: 3,
	}
	suite.createConfigFile(configData)

	os.Args = []string{"program"}

	cfg, err := ParseCfg()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "ffmpeg", cfg.FfmpegNameStr)
}

// TestParseCfg_FfmpegPath_Unix tests ffmpeg path on Unix-like systems
func (suite *ConfigTestSuite) TestParseCfg_FfmpegPath_Unix() {
	if runtime.GOOS == "windows" {
		suite.T().Skip("Test only runs on non-Windows systems")
	}

	configData := Config{
		Format:      2,
		VideoFormat: 3,
	}
	suite.createConfigFile(configData)

	os.Args = []string{"program"}

	cfg, err := ParseCfg()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "./ffmpeg", cfg.FfmpegNameStr)
}

// TestParseCfg_FfmpegEnvVar tests ffmpeg path when UseFfmpegEnvVar is true
func (suite *ConfigTestSuite) TestParseCfg_FfmpegEnvVar() {
	configData := Config{
		Format:           2,
		VideoFormat:      3,
		UseFfmpegEnvVar: true,
	}
	suite.createConfigFile(configData)

	os.Args = []string{"program"}

	cfg, err := ParseCfg()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "ffmpeg", cfg.FfmpegNameStr)
}

// TestParseCfg_MissingConfigFile tests behavior when config.json doesn't exist
func (suite *ConfigTestSuite) TestParseCfg_MissingConfigFile() {
	// Don't create config file
	os.Args = []string{"program"}

	_, err := ParseCfg()
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to read config")
}

// TestParseCfg_InvalidJSON tests invalid JSON in config file
func (suite *ConfigTestSuite) TestParseCfg_InvalidJSON() {
	// Create invalid JSON config file
	configPath := filepath.Join(suite.tempDir, "config.json")
	err := os.WriteFile(configPath, []byte("invalid json"), 0644)
	assert.NoError(suite.T(), err)

	// Change to temp directory for this test
	oldWd, _ := os.Getwd()
	os.Chdir(suite.tempDir)
	defer os.Chdir(oldWd)

	os.Args = []string{"program"}

	_, err = ParseCfg()
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to read config")
}

// TestProcessUrls_SingleURL tests processing single URL
func (suite *ConfigTestSuite) TestProcessUrls_SingleURL() {
	urls := []string{"https://example.com/album/123/"}

	processed, err := processUrls(urls)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), []string{"https://example.com/album/123"}, processed)
}

// TestProcessUrls_TextFile tests processing URLs from text file
func (suite *ConfigTestSuite) TestProcessUrls_TextFile() {
	// Create a text file with URLs
	txtPath := filepath.Join(suite.tempDir, "urls.txt")
	urlContent := "https://example.com/album/1\n\nhttps://example.com/album/2/\nhttps://example.com/album/1\n"
	err := os.WriteFile(txtPath, []byte(urlContent), 0644)
	assert.NoError(suite.T(), err)

	urls := []string{txtPath}

	processed, err := processUrls(urls)

	assert.NoError(suite.T(), err)
	// Should deduplicate and trim slashes
	expected := []string{"https://example.com/album/1", "https://example.com/album/2"}
	assert.Equal(suite.T(), expected, processed)
}

// TestProcessUrls_Mixed tests processing mix of URLs and text files
func (suite *ConfigTestSuite) TestProcessUrls_Mixed() {
	// Create a text file with URLs
	txtPath := filepath.Join(suite.tempDir, "urls.txt")
	urlContent := "https://example.com/album/2\nhttps://example.com/album/3"
	err := os.WriteFile(txtPath, []byte(urlContent), 0644)
	assert.NoError(suite.T(), err)

	urls := []string{"https://example.com/album/1", txtPath, "https://example.com/album/2"}

	processed, err := processUrls(urls)

	assert.NoError(suite.T(), err)
	// Should deduplicate
	expected := []string{"https://example.com/album/1", "https://example.com/album/2", "https://example.com/album/3"}
	assert.Equal(suite.T(), expected, processed)
}

// TestProcessUrls_NonExistentFile tests processing URLs with non-existent text file
func (suite *ConfigTestSuite) TestProcessUrls_NonExistentFile() {
	urls := []string{"https://example.com/album/1", "nonexistent.txt"}

	_, err := processUrls(urls)

	assert.Error(suite.T(), err)
}

// TestContains tests the contains helper function
func (suite *ConfigTestSuite) TestContains() {
	slice := []string{"item1", "Item2", "ITEM3"}

	// Test case-insensitive matching
	assert.True(suite.T(), contains(slice, "item1"))
	assert.True(suite.T(), contains(slice, "ITEM1"))
	assert.True(suite.T(), contains(slice, "item2"))
	assert.True(suite.T(), contains(slice, "Item3"))
	assert.False(suite.T(), contains(slice, "item4"))
}

// TestReadConfig_Valid tests reading valid config file
func (suite *ConfigTestSuite) TestReadConfig_Valid() {
	configData := Config{
		Email:    "test@example.com",
		Password: "testpass",
		Format:   2,
	}
	suite.createConfigFile(configData)

	// Change to temp directory
	oldWd, _ := os.Getwd()
	os.Chdir(suite.tempDir)
	defer os.Chdir(oldWd)

	cfg, err := readConfig()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "test@example.com", cfg.Email)
	assert.Equal(suite.T(), "testpass", cfg.Password)
	assert.Equal(suite.T(), 2, cfg.Format)
}

// TestReadConfig_NonExistent tests reading non-existent config file
func (suite *ConfigTestSuite) TestReadConfig_NonExistent() {
	// Change to temp directory (no config.json)
	oldWd, _ := os.Getwd()
	os.Chdir(suite.tempDir)
	defer os.Chdir(oldWd)

	_, err := readConfig()
	assert.Error(suite.T(), err)
}

// TestParseArgs tests command line argument parsing
func (suite *ConfigTestSuite) TestParseArgs() {
	os.Args = []string{
		"program",
		"-f", "3",
		"-v", "4",
		"-o", "output/dir",
		"--force-video",
		"--skip-videos",
		"https://example.com/1",
		"https://example.com/2",
	}

	args := parseArgs()

	assert.NotNil(suite.T(), args)
	assert.Equal(suite.T(), 3, *args.Format)
	assert.Equal(suite.T(), 4, *args.VideoFormat)
	assert.Equal(suite.T(), "output/dir", args.OutPath)
	assert.True(suite.T(), args.ForceVideo)
	assert.True(suite.T(), args.SkipVideos)
	assert.Equal(suite.T(), []string{"https://example.com/1", "https://example.com/2"}, args.Urls)
}

// TestConstants tests that constants are properly defined
func (suite *ConfigTestSuite) TestConstants() {
	assert.Equal(suite.T(), 1, MinAudioFormat)
	assert.Equal(suite.T(), 5, MaxAudioFormat)
	assert.Equal(suite.T(), 1, MinVideoFormat)
	assert.Equal(suite.T(), 5, MaxVideoFormat)
}

// TestResolveRes tests resolution mapping
func (suite *ConfigTestSuite) TestResolveRes() {
	assert.Equal(suite.T(), "480", resolveRes[1])
	assert.Equal(suite.T(), "720", resolveRes[2])
	assert.Equal(suite.T(), "1080", resolveRes[3])
	assert.Equal(suite.T(), "1440", resolveRes[4])
	assert.Equal(suite.T(), "2160", resolveRes[5])
}

// Helper method to create config.json file
func (suite *ConfigTestSuite) createConfigFile(cfg Config) {
	configPath := filepath.Join(suite.tempDir, "config.json")

	data, err := json.Marshal(cfg)
	assert.NoError(suite.T(), err)

	err = os.WriteFile(configPath, data, 0644)
	assert.NoError(suite.T(), err)

	// Change to temp directory so ParseCfg can find config.json
	oldWd, _ := os.Getwd()
	os.Chdir(suite.tempDir)
	suite.T().Cleanup(func() {
		os.Chdir(oldWd)
	})
}

// Run the test suite
func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}
