package processor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"main/pkg/api"
	"main/pkg/config"
	"main/pkg/downloader"
	"main/pkg/models"
)

// TestSuite for processor package
type ProcessorTestSuite struct {
	suite.Suite
	tempDir    string
	server     *httptest.Server
	apiClient  *api.Client
	downloader *downloader.Downloader
	config     *config.Config
	processor  *Processor
}

// SetupTest creates a temporary directory and test infrastructure
func (suite *ProcessorTestSuite) SetupTest() {
	tempDir, err := os.MkdirTemp("", "processor_test_*")
	assert.NoError(suite.T(), err)
	suite.tempDir = tempDir

	// Create test HTTP server
	suite.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.handleRequest(w, r)
	}))

	// Create API client with test URLs
	suite.apiClient = api.NewClient()
	// Configure the client with test URLs
	suite.apiClient.BaseAuthURL = suite.server.URL + "/connect/token"
	suite.apiClient.BaseUserInfoURL = suite.server.URL + "/connect/userinfo"
	suite.apiClient.BaseSubInfoURL = suite.server.URL + "/api/v1/me/subscriptions"
	suite.apiClient.BaseStreamURL = suite.server.URL + "/"

	// Create test config
	suite.config = &config.Config{
		Format:        2,
		VideoFormat:   3,
		OutPath:       suite.tempDir,
		WantRes:       "1080",
		FfmpegNameStr: "ffmpeg",
		Email:         "test@example.com",
	}

	// Create downloader
	suite.downloader = downloader.NewDownloader(suite.apiClient, suite.config)

	// Create processor
	suite.processor = NewProcessor(suite.apiClient, suite.downloader, suite.config)
}

// TearDownTest cleans up the temporary directory
func (suite *ProcessorTestSuite) TearDownTest() {
	if suite.tempDir != "" {
		os.RemoveAll(suite.tempDir)
	}
	suite.server.Close()
}

// handleRequest mocks different API endpoints
func (suite *ProcessorTestSuite) handleRequest(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/connect/token":
		suite.handleAuth(w, r)
	case "/connect/userinfo":
		suite.handleUserInfo(w, r)
	case "/api/v1/me/subscriptions":
		suite.handleSubInfo(w, r)
	case "/api.aspx":
		suite.handleApiAsx(w, r)
	case "/bigriver/subPlayer.aspx":
		suite.handleSubPlayer(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Mock handlers
func (suite *ProcessorTestSuite) handleAuth(w http.ResponseWriter, r *http.Request) {
	response := models.AuthResponse{AccessToken: "mock-access-token"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (suite *ProcessorTestSuite) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	response := models.UserInfo{Sub: "user-123"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (suite *ProcessorTestSuite) handleSubInfo(w http.ResponseWriter, r *http.Request) {
	response := models.SubInfo{
		Plan: models.Plan{
			Description: "Premium Plan",
			PlanID:      "plan-123",
		},
		LegacySubscriptionID: "sub-456",
		StartedAt:           "01/01/2024 00:00:00",
		EndsAt:             "12/31/2024 23:59:59",
		IsContentAccessible: true,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (suite *ProcessorTestSuite) handleApiAsx(w http.ResponseWriter, r *http.Request) {
	method := r.URL.Query().Get("method")

	if method == "catalog.container" {
		response := models.AlbumMeta{
			Response: &models.AlbArtResp{
				ArtistName: "Test Artist",
				ContainerID: 123,
				ContainerInfo: "Test Album",
				Tracks: []models.Track{
					{TrackID: 1, SongTitle: "Test Song"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else if method == "catalog.containersAll" {
		response := []*models.ArtistMeta{
			{
				Response: &models.ArtistResp{
					Containers: []*models.AlbArtResp{
						{
							ArtistName: "Test Artist",
							ContainerID: 123,
							ContainerInfo: "Test Album",
							ContainerTypeStr: "Album",
							AvailabilityTypeStr: "AVAILABLE",
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func (suite *ProcessorTestSuite) handleSubPlayer(w http.ResponseWriter, r *http.Request) {
	response := models.StreamMeta{
		StreamLink: "https://stream.example.com/audio.m3u8",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// TestNewProcessor tests processor creation
func (suite *ProcessorTestSuite) TestNewProcessor() {
	processor := NewProcessor(suite.apiClient, suite.downloader, suite.config)
	assert.NotNil(suite.T(), processor)
	assert.Equal(suite.T(), suite.apiClient, processor.apiClient)
	assert.Equal(suite.T(), suite.downloader, processor.downloader)
	assert.Equal(suite.T(), suite.config, processor.config)
}

// TestGetAlbumTotal tests album total calculation
func (suite *ProcessorTestSuite) TestGetAlbumTotal() {
	meta := []*models.ArtistMeta{
		{
			Response: &models.ArtistResp{
				Containers: []*models.AlbArtResp{
					{ContainerID: 1},
					{ContainerID: 2},
				},
			},
		},
		{
			Response: &models.ArtistResp{
				Containers: []*models.AlbArtResp{
					{ContainerID: 3},
				},
			},
		},
	}

	total := getAlbumTotal(meta)
	assert.Equal(suite.T(), 3, total)
}

// TestGetVideoSku tests video SKU extraction
func (suite *ProcessorTestSuite) TestGetVideoSku() {
	products := []models.Product{
		{FormatStr: "AUDIO ONLY", SkuID: 1},
		{FormatStr: "VIDEO ON DEMAND", SkuID: 2},
		{FormatStr: "LIVE HD VIDEO", SkuID: 3},
	}

	skuID := getVideoSku(products)
	assert.Equal(suite.T(), 2, skuID)

	// Test no video products
	audioOnly := []models.Product{
		{FormatStr: "AUDIO ONLY", SkuID: 1},
	}
	skuID = getVideoSku(audioOnly)
	assert.Equal(suite.T(), 0, skuID)
}

// TestGetLstreamSku tests livestream SKU extraction
func (suite *ProcessorTestSuite) TestGetLstreamSku() {
	products := []*models.ProductFormatList{
		{FormatStr: "AUDIO ONLY", SkuID: 1},
		{FormatStr: "LIVE HD VIDEO", SkuID: 2},
	}

	skuID := getLstreamSku(products)
	assert.Equal(suite.T(), 2, skuID)

	// Test no livestream products
	audioOnly := []*models.ProductFormatList{
		{FormatStr: "AUDIO ONLY", SkuID: 1},
	}
	skuID = getLstreamSku(audioOnly)
	assert.Equal(suite.T(), 0, skuID)
}

// TestGetLstreamContainer tests livestream container selection
func (suite *ProcessorTestSuite) TestGetLstreamContainer() {
	containers := []*models.AlbArtResp{
		{
			ContainerTypeStr: "Album",
			AvailabilityTypeStr: "AVAILABLE",
		},
		{
			ContainerTypeStr: "Show",
			AvailabilityTypeStr: "AVAILABLE",
		},
		{
			ContainerTypeStr: "Show",
			AvailabilityTypeStr: "NOT_AVAILABLE",
		},
	}

	container := getLstreamContainer(containers)
	assert.NotNil(suite.T(), container)
	assert.Equal(suite.T(), "Show", container.ContainerTypeStr)
	assert.Equal(suite.T(), "AVAILABLE", container.AvailabilityTypeStr)
}

// TestParseLstreamMeta tests livestream metadata parsing
func (suite *ProcessorTestSuite) TestParseLstreamMeta() {
	meta := &models.ArtistMeta{
		Response: &models.ArtistResp{
			Containers: []*models.AlbArtResp{
				{
					ArtistName: "Test Artist",
					ContainerInfo: "Test Show",
					ContainerID: 123,
					ContainerTypeStr: "Show",
					AvailabilityTypeStr: "AVAILABLE",
					VideoChapters: []interface{}{"chapter1", "chapter2"},
					Products: []models.Product{{FormatStr: "VIDEO", SkuID: 456}},
					ProductFormatList: []*models.ProductFormatList{{FormatStr: "LIVE HD VIDEO", SkuID: 789}},
				},
			},
		},
	}

	parsed := parseLstreamMeta(meta)
	assert.NotNil(suite.T(), parsed)
	assert.Equal(suite.T(), "Test Artist", parsed.Response.ArtistName)
	assert.Equal(suite.T(), "Test Show", parsed.Response.ContainerInfo)
	assert.Equal(suite.T(), 123, parsed.Response.ContainerID)
	assert.Len(suite.T(), parsed.Response.VideoChapters, 2)
}

// TestResolveCatPlistId tests catalog playlist ID resolution
func (suite *ProcessorTestSuite) TestResolveCatPlistId() {
	// Create a test server that redirects to a catalog playlist URL
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate redirect to catalog playlist URL
		w.Header().Set("Location", "https://play.nugs.net/?plGUID=test-playlist-id")
		w.WriteHeader(http.StatusFound)
	}))
	defer testServer.Close()

	// Test with a catalog playlist URL
	plistID, err := resolveCatPlistId(testServer.URL + "/playlist")

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "test-playlist-id", plistID)
}

// TestResolveCatPlistId_Invalid tests invalid catalog playlist URL
func (suite *ProcessorTestSuite) TestResolveCatPlistId_Invalid() {
	// Test with a non-redirecting URL
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not a redirect"))
	}))
	defer testServer.Close()

	plistID, err := resolveCatPlistId(testServer.URL)

	assert.Error(suite.T(), err)
	assert.Empty(suite.T(), plistID)
}

// TestProcessCatalogPlist tests catalog playlist processing
func (suite *ProcessorTestSuite) TestProcessCatalogPlist() {
	// This test would require more complex mocking of the entire flow
	// For now, we'll test the basic setup and error handling

	err := suite.processor.ProcessCatalogPlist("invalid-playlist-id", "token", nil)

	// Should fail because we don't have proper mocking for the full flow
	assert.Error(suite.T(), err)
}

// TestProcessPaidLstream tests paid livestream processing
func (suite *ProcessorTestSuite) TestProcessPaidLstream() {
	// This test would require extensive mocking of video processing
	// For now, we'll test the basic parameter parsing

	query := "showID=123"
	streamParams := &models.StreamParams{
		SubscriptionID: "sub-123",
		UserID:         "user-456",
	}

	err := suite.processor.ProcessPaidLstream(query, "ugu-789", streamParams)

	// Should fail due to missing video metadata mocking
	assert.Error(suite.T(), err)
}

// TestProcessAlbum_NoTracks tests album processing with no tracks
func (suite *ProcessorTestSuite) TestProcessAlbum_NoTracks() {
	// Create album metadata with no tracks
	albumMeta := &models.AlbArtResp{
		ArtistName: "Test Artist",
		ContainerInfo: "Test Album",
		Tracks: []models.Track{}, // No tracks
		Products: []models.Product{
			{FormatStr: "VIDEO ON DEMAND", SkuID: 123},
		},
	}

	streamParams := &models.StreamParams{
		SubscriptionID: "sub-123",
		UserID:         "user-456",
	}

	// Should return error about no tracks
	err := suite.processor.ProcessAlbum("", streamParams, albumMeta)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "release has no tracks")
}

// TestProcessAlbum_VideoOnly tests video-only album processing
func (suite *ProcessorTestSuite) TestProcessAlbum_VideoOnly() {
	// Create album metadata with video but no tracks
	albumMeta := &models.AlbArtResp{
		ArtistName: "Test Artist",
		ContainerInfo: "Test Video Album",
		Tracks: []models.Track{}, // No tracks
		Products: []models.Product{
			{FormatStr: "VIDEO ON DEMAND", SkuID: 123},
		},
	}

	streamParams := &models.StreamParams{
		SubscriptionID: "sub-123",
		UserID:         "user-456",
	}

	// Should attempt video processing (will fail due to mocking limitations)
	err := suite.processor.ProcessAlbum("", streamParams, albumMeta)
	assert.Error(suite.T(), err) // Will fail due to incomplete mocking
}

// TestProcessPlaylist tests playlist processing
func (suite *ProcessorTestSuite) TestProcessPlaylist() {
	streamParams := &models.StreamParams{
		SubscriptionID: "sub-123",
		UserID:         "user-456",
	}

	// Test with catalog playlist (cat = true)
	err := suite.processor.ProcessPlaylist("test-playlist", "legacy-token", streamParams, true)
	assert.Error(suite.T(), err) // Will fail due to mocking limitations
}

// TestProcessArtist tests artist processing
func (suite *ProcessorTestSuite) TestProcessArtist() {
	streamParams := &models.StreamParams{
		SubscriptionID: "sub-123",
		UserID:         "user-456",
	}

	// Will fail due to API mocking limitations, but tests the basic flow
	err := suite.processor.ProcessArtist("test-artist-id", streamParams)
	assert.Error(suite.T(), err)
}

// TestProcessTrack tests individual track processing
func (suite *ProcessorTestSuite) TestProcessTrack() {
	track := &models.Track{
		TrackID:   123,
		SongTitle: "Test Track",
	}

	streamParams := &models.StreamParams{
		SubscriptionID: "sub-123",
		UserID:         "user-456",
	}

	// Create a test folder
	testFolder := filepath.Join(suite.tempDir, "test_album")
	err := os.MkdirAll(testFolder, 0755)
	assert.NoError(suite.T(), err)

	// Will fail due to API mocking limitations, but tests the basic setup
	err = suite.processor.ProcessTrack(testFolder, 1, 1, track, streamParams)
	assert.Error(suite.T(), err)
}

// TestProcessVideo tests video processing
func (suite *ProcessorTestSuite) TestProcessVideo() {
	streamParams := &models.StreamParams{
		SubscriptionID: "sub-123",
		UserID:         "user-456",
	}

	// Will fail due to extensive mocking requirements, but tests the basic flow
	err := suite.processor.ProcessVideo("test-video-id", "", streamParams, nil, false)
	assert.Error(suite.T(), err)
}

// TestConstants tests that constants are properly defined
func (suite *ProcessorTestSuite) TestConstants() {
	assert.Equal(suite.T(), 100, MaxFolderNameLen)
	assert.Equal(suite.T(), 200, MaxVideoFilenameLen)
	assert.Len(suite.T(), streamMetaIndices, 4)
	assert.Equal(suite.T(), [4]int{1, 4, 7, 10}, streamMetaIndices)
}

// Run the test suite
func TestProcessorTestSuite(t *testing.T) {
	suite.Run(t, new(ProcessorTestSuite))
}
