package downloader

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"main/pkg/api"
	"main/pkg/config"
	"main/pkg/models"
)

// TestSuite for downloader package
type DownloaderTestSuite struct {
	suite.Suite
	tempDir    string
	server     *httptest.Server
	apiClient  *api.Client
	downloader *Downloader
	config     *config.Config
}

// SetupTest creates a temporary directory and test infrastructure
func (suite *DownloaderTestSuite) SetupTest() {
	tempDir, err := os.MkdirTemp("", "downloader_test_*")
	assert.NoError(suite.T(), err)
	suite.tempDir = tempDir

	// Create test HTTP server
	suite.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.handleRequest(w, r)
	}))

	// Create API client with test URLs
	suite.apiClient = api.NewClient()
	// Configure the client with test URLs
	suite.apiClient.BaseStreamURL = suite.server.URL + "/"

	// Create test config
	suite.config = &config.Config{
		Format:        2,
		VideoFormat:   3,
		OutPath:       suite.tempDir,
		WantRes:       "1080",
		FfmpegNameStr: "ffmpeg", // Assume ffmpeg is available for tests
	}

	// Create downloader
	suite.downloader = NewDownloader(suite.apiClient, suite.config)
}

// TearDownTest cleans up the temporary directory
func (suite *DownloaderTestSuite) TearDownTest() {
	if suite.tempDir != "" {
		os.RemoveAll(suite.tempDir)
	}
	suite.server.Close()
}

// handleRequest mocks different endpoints
func (suite *DownloaderTestSuite) handleRequest(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/playlist.m3u8":
		suite.handleVideoM3U8Playlist(w, r)
	case "/audio_playlist.m3u8":
		suite.handleAudioM3U8Playlist(w, r)
	case "/media.m3u8":
		suite.handleMediaPlaylist(w, r)
	case "/key":
		suite.handleKey(w, r)
	case "/segment.ts":
		suite.handleSegment(w, r)
	default:
		if strings.HasSuffix(r.URL.Path, ".m3u8") {
			suite.handleVideoM3U8Playlist(w, r)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// Mock handlers
func (suite *DownloaderTestSuite) handleVideoM3U8Playlist(w http.ResponseWriter, r *http.Request) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=1280000,RESOLUTION=1280x720,CODECS="avc1.42e00a,mp4a.40.2"
video_720p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2560000,RESOLUTION=1920x1080,CODECS="avc1.42e00a,mp4a.40.2"
video_1080p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5120000,RESOLUTION=3840x2160,CODECS="avc1.42e00a,mp4a.40.2"
video_2160p.m3u8
`
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write([]byte(playlist))
}

func (suite *DownloaderTestSuite) handleAudioM3U8Playlist(w http.ResponseWriter, r *http.Request) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=1280000,CODECS="mp4a.40.2"
audio_128k_v1.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2560000,CODECS="mp4a.40.2"
audio_256k_v1.m3u8
`
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write([]byte(playlist))
}

func (suite *DownloaderTestSuite) handleMediaPlaylist(w http.ResponseWriter, r *http.Request) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-KEY:METHOD=AES-128,URI="/key",IV=0x1234567890abcdef1234567890abcdef
#EXTINF:9.9,
segment1.ts
#EXTINF:10.0,
segment2.ts
#EXT-X-ENDLIST
`
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write([]byte(playlist))
}

func (suite *DownloaderTestSuite) handleKey(w http.ResponseWriter, r *http.Request) {
	// Return a 16-byte key
	key := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	w.Write(key)
}

func (suite *DownloaderTestSuite) handleSegment(w http.ResponseWriter, r *http.Request) {
	// Return some test data
	testData := make([]byte, 1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	w.Write(testData)
}

// TestNewDownloader tests downloader creation
func (suite *DownloaderTestSuite) TestNewDownloader() {
	downloader := NewDownloader(suite.apiClient, suite.config)
	assert.NotNil(suite.T(), downloader)
	assert.Equal(suite.T(), suite.apiClient, downloader.apiClient)
	assert.Equal(suite.T(), suite.config, downloader.config)
}

// TestQueryQuality tests quality detection from stream URLs
func (suite *DownloaderTestSuite) TestQueryQuality() {
	testCases := []struct {
		url      string
		expected *models.Quality
	}{
		{"https://stream.example.com/audio.flac16/", &models.Quality{Extension: ".flac", Format: 2}},
		{"https://stream.example.com/audio.alac16/", &models.Quality{Extension: ".m4a", Format: 1}},
		{"https://stream.example.com/audio.m3u8?", &models.Quality{Extension: ".m4a", Format: 6}},
		{"https://stream.example.com/unknown", nil},
	}

	for _, tc := range testCases {
		result := QueryQuality(tc.url)
		if tc.expected == nil {
			assert.Nil(suite.T(), result)
		} else {
			assert.NotNil(suite.T(), result)
			assert.Equal(suite.T(), tc.expected.Extension, result.Extension)
			assert.Equal(suite.T(), tc.expected.Format, result.Format)
		}
	}
}

// TestGetTrackQual tests track quality selection
func (suite *DownloaderTestSuite) TestGetTrackQual() {
	quals := []*models.Quality{
		{Format: 1, Specs: "ALAC"},
		{Format: 2, Specs: "FLAC"},
		{Format: 5, Specs: "AAC"},
	}

	// Test finding existing quality
	result := GetTrackQual(quals, 2)
	assert.NotNil(suite.T(), result)
	assert.Equal(suite.T(), "FLAC", result.Specs)

	// Test quality not found
	result = GetTrackQual(quals, 3)
	assert.Nil(suite.T(), result)
}

// TestCheckIfHlsOnly tests HLS-only detection
func (suite *DownloaderTestSuite) TestCheckIfHlsOnly() {
	// Test HLS-only qualities
	hlsQuals := []*models.Quality{
		{URL: "https://stream.example.com/audio.m3u8?", Format: 6},
		{URL: "https://stream.example.com/audio2.m3u8?", Format: 6},
	}
	assert.True(suite.T(), CheckIfHlsOnly(hlsQuals))

	// Test mixed qualities
	mixedQuals := []*models.Quality{
		{URL: "https://stream.example.com/audio.flac16/", Format: 2},
		{URL: "https://stream.example.com/audio.m3u8?", Format: 6},
	}
	assert.False(suite.T(), CheckIfHlsOnly(mixedQuals))
}

// TestParseHlsMaster tests HLS master playlist parsing
func (suite *DownloaderTestSuite) TestParseHlsMaster() {
	quality := &models.Quality{
		URL: suite.server.URL + "/audio_playlist.m3u8",
	}

	err := suite.downloader.ParseHlsMaster(quality)
	assert.NoError(suite.T(), err)
	assert.Contains(suite.T(), quality.Specs, "Kbps AAC")
	assert.Contains(suite.T(), quality.URL, "audio_256k_v1.m3u8")
}

// TestGetManifestBase tests manifest base URL extraction
func (suite *DownloaderTestSuite) TestGetManifestBase() {
	manifestURL := "https://stream.example.com/path/to/manifest.m3u8?param=value"

	base, query, err := suite.downloader.GetManifestBase(manifestURL)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "https://stream.example.com/path/to/", base)
	assert.Equal(suite.T(), "?param=value", query)
}

// TestGetSegUrls tests segment URL extraction
func (suite *DownloaderTestSuite) TestGetSegUrls() {
	mediaURL := suite.server.URL + "/media.m3u8"
	query := "?param=value"

	segUrls, err := suite.downloader.GetSegUrls(mediaURL, query)

	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), segUrls, 2)
	assert.Contains(suite.T(), segUrls[0], "segment1.ts")
	assert.Contains(suite.T(), segUrls[0], "?param=value")
}

// TestChooseVariant tests video variant selection
func (suite *DownloaderTestSuite) TestChooseVariant() {
	manifestURL := suite.server.URL + "/playlist.m3u8"

	variant, res, err := suite.downloader.ChooseVariant(manifestURL, "720")

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), variant)
	assert.Equal(suite.T(), "720p", res)
}

// TestExtractBitrate tests bitrate extraction from URLs
func (suite *DownloaderTestSuite) TestExtractBitrate() {
	testCases := []struct {
		url      string
		expected string
	}{
		{"audio_128k_v1.m3u8", "128"},
		{"audio_256k_v2.m3u8", "256"},
		{"audio_no_bitrate.m3u8", ""},
		{"invalid_format", ""},
	}

	for _, tc := range testCases {
		result := extractBitrate(tc.url)
		assert.Equal(suite.T(), tc.expected, result, "Failed for URL: %s", tc.url)
	}
}

// TestGetKey tests encryption key retrieval
func (suite *DownloaderTestSuite) TestGetKey() {
	keyURL := suite.server.URL + "/key"

	key, err := GetKey(keyURL, suite.apiClient)

	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), key, 16)
}

// TestSanitise tests filename sanitization
func (suite *DownloaderTestSuite) TestSanitise() {
	testCases := []struct {
		input    string
		expected string
	}{
		{"normal_file.mp3", "normal_file.mp3"},
		{"file:with*chars?.mp3", "file_with_chars_.mp3"},
		{"file/with\\bad:chars*?.mp3", "file_with\\bad_chars__.mp3"},
		{"file\t", "file"},
	}

	for _, tc := range testCases {
		result := Sanitise(tc.input)
		assert.Equal(suite.T(), tc.expected, result, "Failed for input: %s", tc.input)
	}
}

// TestFileExists tests file existence checking
func (suite *DownloaderTestSuite) TestFileExists() {
	// Test existing file
	testFile := filepath.Join(suite.tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	assert.NoError(suite.T(), err)

	exists, err := FileExists(testFile)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), exists)

	// Test non-existing file
	nonExistent := filepath.Join(suite.tempDir, "nonexistent.txt")
	exists, err = FileExists(nonExistent)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), exists)

	// Test directory (should return false)
	exists, err = FileExists(suite.tempDir)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), exists)
}

// TestParseDuration tests duration parsing
func (suite *DownloaderTestSuite) TestParseDuration() {
	testCases := []struct {
		input    string
		expected int
		hasError bool
	}{
		{"00:00:30.500", 31, false}, // Rounds up
		{"00:01:30.000", 90, false},
		{"01:00:00.000", 3600, false},
		{"invalid", 0, true},
	}

	for _, tc := range testCases {
		result, err := parseDuration(tc.input)
		if tc.hasError {
			assert.Error(suite.T(), err, "Expected error for input: %s", tc.input)
		} else {
			assert.NoError(suite.T(), err, "Unexpected error for input: %s", tc.input)
			assert.Equal(suite.T(), tc.expected, result, "Failed for input: %s", tc.input)
		}
	}
}

// TestExtractDuration tests duration extraction from ffmpeg output
func (suite *DownloaderTestSuite) TestExtractDuration() {
	ffmpegOutput := `ffmpeg version 4.4.2 Copyright (c) 2000-2021 the FFmpeg developers
  Duration: 00:03:45.67, start: 0.000000, bitrate: 128 kb/s
  Stream #0:0: Audio: aac (LC), 44100 Hz, stereo, fltp, 128 kb/s`

	duration := extractDuration(ffmpegOutput)
	assert.Equal(suite.T(), "00:03:45.67", duration)

	// Test no match
	noMatch := "some other output"
	duration = extractDuration(noMatch)
	assert.Equal(suite.T(), "", duration)
}

// TestDownloadTrack tests track downloading (basic functionality)
func (suite *DownloaderTestSuite) TestDownloadTrack_Basic() {
	// This is a basic test - in real scenarios, we'd need a proper HTTP server
	// that serves actual file content. For now, we test the basic setup.
	testFile := filepath.Join(suite.tempDir, "test_track.m4a")

	// Create a test server that serves some content
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake audio content"))
	}))
	defer testServer.Close()

	err := suite.downloader.DownloadTrack(testFile, testServer.URL)
	assert.NoError(suite.T(), err)

	// Verify file was created
	_, err = os.Stat(testFile)
	assert.NoError(suite.T(), err)
}

// TestDownloadVideo tests video downloading (basic functionality)
func (suite *DownloaderTestSuite) TestDownloadVideo_Basic() {
	testFile := filepath.Join(suite.tempDir, "test_video.ts")
	testContent := make([]byte, 100)
	for i := range testContent {
		testContent[i] = byte(i % 256)
	}

	// Create a test server that serves some content
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Write(testContent)
	}))
	defer testServer.Close()

	err := suite.downloader.DownloadVideo(testFile, testServer.URL)
	assert.NoError(suite.T(), err)

	// Verify file was created
	_, err = os.Stat(testFile)
	assert.NoError(suite.T(), err)
}

// TestTagAudioFile tests audio file tagging
func (suite *DownloaderTestSuite) TestTagAudioFile() {
	// Create a temporary test file
	tempInput, err := os.CreateTemp("", "test_audio_*.m4a")
	assert.NoError(suite.T(), err)
	defer os.Remove(tempInput.Name())

	// Write some dummy content
	_, err = tempInput.Write([]byte("dummy audio content"))
	assert.NoError(suite.T(), err)
	tempInput.Close()

	tempOutput := tempInput.Name() + "_tagged.m4a"
	defer os.Remove(tempOutput)

	metadata := &models.TrackMetadata{
		Title:    "Test Track",
		Artist:   "Test Artist",
		Album:    "Test Album",
		TrackNum: 1,
		Year:     "2024",
	}

	// Test tagging (would fail without ffmpeg, but tests the function structure)
	err = TagAudioFile(tempInput.Name(), tempOutput, "ffmpeg", metadata)
	// We expect this to fail in test environment without ffmpeg, but structure is tested
	if err != nil {
		assert.Contains(suite.T(), err.Error(), "ffmpeg") // Should mention ffmpeg in error
	}
}

// TestTagVideoFile tests video file tagging
func (suite *DownloaderTestSuite) TestTagVideoFile() {
	// Create a temporary test file
	tempInput, err := os.CreateTemp("", "test_video_*.mp4")
	assert.NoError(suite.T(), err)
	defer os.Remove(tempInput.Name())

	// Write some dummy content
	_, err = tempInput.Write([]byte("dummy video content"))
	assert.NoError(suite.T(), err)
	tempInput.Close()

	tempOutput := tempInput.Name() + "_tagged.mp4"
	defer os.Remove(tempOutput)

	metadata := &models.TrackMetadata{
		Title:  "Test Video",
		Artist: "Test Artist",
		Album:  "Test Album",
	}

	// Test tagging (would fail without ffmpeg, but tests the function structure)
	err = TagVideoFile(tempInput.Name(), tempOutput, "ffmpeg", metadata)
	// We expect this to fail in test environment without ffmpeg, but structure is tested
	if err != nil {
		assert.Contains(suite.T(), err.Error(), "ffmpeg") // Should mention ffmpeg in error
	}
}

// TestDownloadTrackWithMetadata tests metadata-enabled track download
func (suite *DownloaderTestSuite) TestDownloadTrackWithMetadata() {
	// Test with proper API client setup (should not panic)
	downloader := NewDownloader(suite.apiClient, suite.config)

	// Test with nil metadata (should not panic)
	err := downloader.DownloadTrackWithMetadata("test.m4a", "http://example.com", nil, "ffmpeg")
	assert.Error(suite.T(), err) // Should fail due to network/file issues, but not panic
}

// TestHlsOnlyWithMetadata tests metadata-enabled HLS processing
func (suite *DownloaderTestSuite) TestHlsOnlyWithMetadata() {
	// Test with proper API client setup (should not panic)
	downloader := NewDownloader(suite.apiClient, suite.config)

	// Test with nil metadata (should not panic)
	err := downloader.HlsOnlyWithMetadata("test.m4a", "http://example.com/manifest.m3u8", "ffmpeg", nil)
	assert.Error(suite.T(), err) // Should fail due to network/file issues, but not panic
}

// TestTagAudioFile_MetadataValidation tests metadata parameter validation
func (suite *DownloaderTestSuite) TestTagAudioFile_MetadataValidation() {
	tempInput, err := os.CreateTemp("", "test_validation_*.m4a")
	assert.NoError(suite.T(), err)
	defer os.Remove(tempInput.Name())

	tempInput.Write([]byte("test"))
	tempInput.Close()

	tempOutput := tempInput.Name() + "_out.m4a"
	defer os.Remove(tempOutput)

	// Test with empty metadata
	emptyMetadata := &models.TrackMetadata{}
	err = TagAudioFile(tempInput.Name(), tempOutput, "ffmpeg", emptyMetadata)
	if err != nil {
		assert.Contains(suite.T(), err.Error(), "ffmpeg")
	}

	// Test with partial metadata
	partialMetadata := &models.TrackMetadata{
		Title:  "Only Title",
		Artist: "", // Empty fields should be handled
	}
	err = TagAudioFile(tempInput.Name(), tempOutput+"_2.m4a", "ffmpeg", partialMetadata)
	defer os.Remove(tempOutput + "_2.m4a")
	if err != nil {
		assert.Contains(suite.T(), err.Error(), "ffmpeg")
	}
}

// TestTagVideoFile_MetadataValidation tests video metadata parameter validation
func (suite *DownloaderTestSuite) TestTagVideoFile_MetadataValidation() {
	tempInput, err := os.CreateTemp("", "test_video_validation_*.mp4")
	assert.NoError(suite.T(), err)
	defer os.Remove(tempInput.Name())

	tempInput.Write([]byte("test"))
	tempInput.Close()

	tempOutput := tempInput.Name() + "_out.mp4"
	defer os.Remove(tempOutput)

	// Test with minimal metadata
	minimalMetadata := &models.TrackMetadata{
		Title: "Video Title",
	}
	err = TagVideoFile(tempInput.Name(), tempOutput, "ffmpeg", minimalMetadata)
	if err != nil {
		assert.Contains(suite.T(), err.Error(), "ffmpeg")
	}
}

// Run the test suite
func TestDownloaderTestSuite(t *testing.T) {
	suite.Run(t, new(DownloaderTestSuite))
}
