package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"main/pkg/models"
)

// TestSuite for api package
type ApiTestSuite struct {
	suite.Suite
	server *httptest.Server
	client *Client
}

// SetupTest creates a test HTTP server and client
func (suite *ApiTestSuite) SetupTest() {
	suite.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.handleRequest(w, r)
	}))
	suite.client = NewClient()
	// Configure client to use test server URLs
	suite.client.BaseAuthURL = suite.server.URL + "/connect/token"
	suite.client.BaseUserInfoURL = suite.server.URL + "/connect/userinfo"
	suite.client.BaseSubInfoURL = suite.server.URL + "/api/v1/me/subscriptions"
	suite.client.BaseStreamURL = suite.server.URL + "/"
}

// TearDownTest closes the test server
func (suite *ApiTestSuite) TearDownTest() {
	suite.server.Close()
}

// handleRequest mocks different API endpoints
func (suite *ApiTestSuite) handleRequest(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/connect/token":
		suite.handleAuth(w, r)
	case "/connect/userinfo":
		suite.handleUserInfo(w, r)
	case "/api/v1/me/subscriptions":
		suite.handleSubInfo(w, r)
	case "/api.aspx":
		suite.handleApiAsx(w, r)
	case "/secureApi.aspx":
		suite.handleSecureApiAsx(w, r)
	case "/bigriver/subPlayer.aspx":
		suite.handleSubPlayer(w, r)
	case "/bigriver/vidPlayer.aspx":
		suite.handleVidPlayer(w, r)
	case "/playlist.m3u8":
		suite.handleM3U8Playlist(w, r)
	case "/media.m3u8":
		suite.handleMediaPlaylist(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Mock handlers for different endpoints
func (suite *ApiTestSuite) handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Check basic auth params
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "test@example.com" && password == "testpass" {
		response := models.AuthResponse{AccessToken: "mock-access-token"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else {
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func (suite *ApiTestSuite) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	auth := r.Header.Get("Authorization")
	if auth != "Bearer mock-access-token" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	response := models.UserInfo{Sub: "user-123"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (suite *ApiTestSuite) handleSubInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	auth := r.Header.Get("Authorization")
	if auth != "Bearer mock-access-token" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

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

func (suite *ApiTestSuite) handleApiAsx(w http.ResponseWriter, r *http.Request) {
	method := r.URL.Query().Get("method")
	containerID := r.URL.Query().Get("containerID")
	plGUID := r.URL.Query().Get("plGUID")

	if method == "catalog.container" && containerID == "123" {
		response := models.AlbumMeta{
			Response: &models.AlbArtResp{
				ArtistName: "Test Artist",
				ContainerID: 123,
				Tracks: []models.Track{
					{TrackID: 1, SongTitle: "Test Song"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else if method == "catalog.playlist" && plGUID == "plist-123" {
		response := models.PlistMeta{
			Response: &models.PlistResp{
				PlayListName: "Test Playlist",
				Items: []models.PlistItem{
					{Track: models.Track{TrackID: 1, SongTitle: "Test Song"}},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else {
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (suite *ApiTestSuite) handleSecureApiAsx(w http.ResponseWriter, r *http.Request) {
	response := models.PlistMeta{
		Response: &models.PlistResp{
			PlayListName: "Test Playlist",
			Items: []models.PlistItem{
				{Track: models.Track{TrackID: 1, SongTitle: "Test Song"}},
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (suite *ApiTestSuite) handleSubPlayer(w http.ResponseWriter, r *http.Request) {
	response := models.StreamMeta{
		StreamLink: "https://stream.example.com/audio.m3u8",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (suite *ApiTestSuite) handleVidPlayer(w http.ResponseWriter, r *http.Request) {
	response := models.PurchasedManResp{
		FileURL: "https://purchased.example.com/manifest.m3u8",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (suite *ApiTestSuite) handleM3U8Playlist(w http.ResponseWriter, r *http.Request) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=1280000,CODECS="mp4a.40.2"
audio_128k.m3u8
`
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write([]byte(playlist))
}

func (suite *ApiTestSuite) handleMediaPlaylist(w http.ResponseWriter, r *http.Request) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:9.9,
segment1.ts
#EXT-X-ENDLIST
`
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write([]byte(playlist))
}

// TestNewClient tests client creation
func (suite *ApiTestSuite) TestNewClient() {
	client := NewClient()
	assert.NotNil(suite.T(), client)
	assert.NotNil(suite.T(), client.GetHTTPClient())
}

// TestAuth_Success tests successful authentication
func (suite *ApiTestSuite) TestAuth_Success() {
	token, err := suite.client.Auth("test@example.com", "testpass")

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "mock-access-token", token)
}

// TestAuth_InvalidCredentials tests authentication with invalid credentials
func (suite *ApiTestSuite) TestAuth_InvalidCredentials() {
	token, err := suite.client.Auth("invalid@example.com", "wrongpass")

	assert.Error(suite.T(), err)
	assert.Empty(suite.T(), token)
}

// TestGetUserInfo_Success tests successful user info retrieval
func (suite *ApiTestSuite) TestGetUserInfo_Success() {
	userID, err := suite.client.GetUserInfo("mock-access-token")

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "user-123", userID)
}

// TestGetUserInfo_InvalidToken tests user info with invalid token
func (suite *ApiTestSuite) TestGetUserInfo_InvalidToken() {
	userID, err := suite.client.GetUserInfo("invalid-token")

	assert.Error(suite.T(), err)
	assert.Empty(suite.T(), userID)
}

// TestGetSubInfo_Success tests successful subscription info retrieval
func (suite *ApiTestSuite) TestGetSubInfo_Success() {
	subInfo, err := suite.client.GetSubInfo("mock-access-token")

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), subInfo)
	assert.Equal(suite.T(), "Premium Plan", subInfo.Plan.Description)
	assert.Equal(suite.T(), "sub-456", subInfo.LegacySubscriptionID)
	assert.True(suite.T(), subInfo.IsContentAccessible)
}

// TestGetAlbumMeta_Success tests successful album metadata retrieval
func (suite *ApiTestSuite) TestGetAlbumMeta_Success() {
	albumMeta, err := suite.client.GetAlbumMeta("123")

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), albumMeta)
	assert.Equal(suite.T(), "Test Artist", albumMeta.Response.ArtistName)
	assert.Equal(suite.T(), 123, albumMeta.Response.ContainerID)
	assert.Len(suite.T(), albumMeta.Response.Tracks, 1)
	assert.Equal(suite.T(), "Test Song", albumMeta.Response.Tracks[0].SongTitle)
}

// TestGetPlistMeta_Catalog tests playlist metadata for catalog playlists
func (suite *ApiTestSuite) TestGetPlistMeta_Catalog() {
	plistMeta, err := suite.client.GetPlistMeta("plist-123", "", "", true)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), plistMeta)
	assert.Equal(suite.T(), "Test Playlist", plistMeta.Response.PlayListName)
	assert.Len(suite.T(), plistMeta.Response.Items, 1)
}

// TestGetStreamMeta_Success tests successful stream metadata retrieval
func (suite *ApiTestSuite) TestGetStreamMeta_Success() {
	streamParams := &models.StreamParams{
		SubscriptionID:          "sub-123",
		SubCostplanIDAccessList: "plan-456",
		UserID:                  "user-789",
		StartStamp:              "1704067200",
		EndStamp:                "1735689600",
	}

	streamLink, err := suite.client.GetStreamMeta(123, 456, 2, streamParams)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "https://stream.example.com/audio.m3u8", streamLink)
}

// TestGetPurchasedManUrl_Success tests successful purchased manifest URL retrieval
func (suite *ApiTestSuite) TestGetPurchasedManUrl_Success() {
	manifestURL, err := suite.client.GetPurchasedManUrl(123, "show-456", "user-789", "uguid-101")

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "https://purchased.example.com/manifest.m3u8", manifestURL)
}

// TestDownloadFile_Success tests successful file download
func (suite *ApiTestSuite) TestDownloadFile_Success() {
	testContent := "test file content"
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testContent))
	}))
	defer testServer.Close()

	resp, err := suite.client.DownloadFile(testServer.URL, "")

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), resp)
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// TestDownloadFile_NotFound tests file download with 404
func (suite *ApiTestSuite) TestDownloadFile_NotFound() {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer testServer.Close()

	resp, err := suite.client.DownloadFile(testServer.URL, "")

	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), resp)
}

// TestGetM3U8Playlist_Success tests successful M3U8 playlist retrieval
func (suite *ApiTestSuite) TestGetM3U8Playlist_Success() {
	playlistURL := suite.server.URL + "/playlist.m3u8"

	playlist, err := suite.client.GetM3U8Playlist(playlistURL)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), playlist)
	assert.True(suite.T(), len(playlist.Variants) > 0)
}

// TestGetMediaPlaylist_Success tests successful media playlist retrieval
func (suite *ApiTestSuite) TestGetMediaPlaylist_Success() {
	playlistURL := suite.server.URL + "/media.m3u8"

	playlist, err := suite.client.GetMediaPlaylist(playlistURL)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), playlist)
	assert.True(suite.T(), len(playlist.Segments) > 0)
}

// TestGetM3U8Playlist_InvalidURL tests M3U8 playlist with invalid URL
func (suite *ApiTestSuite) TestGetM3U8Playlist_InvalidURL() {
	playlist, err := suite.client.GetM3U8Playlist("http://invalid-url")

	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), playlist)
}

// TestGetMediaPlaylist_InvalidURL tests media playlist with invalid URL
func (suite *ApiTestSuite) TestGetMediaPlaylist_InvalidURL() {
	playlist, err := suite.client.GetMediaPlaylist("http://invalid-url")

	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), playlist)
}

// TestConstants tests that constants are properly defined
func (suite *ApiTestSuite) TestConstants() {
	assert.Equal(suite.T(), "x7f54tgbdyc64y656thy47er4", devKey)
	assert.Equal(suite.T(), "Eg7HuH873H65r5rt325UytR5429", clientId)
	assert.Contains(suite.T(), userAgent, "NugsNet")
	assert.Contains(suite.T(), userAgentTwo, "nugsnetAndroid")
	assert.Contains(suite.T(), authUrl, "id.nugs.net")
	assert.Contains(suite.T(), streamApiBase, "streamapi.nugs.net")
}

// Run the test suite
func TestApiTestSuite(t *testing.T) {
	suite.Run(t, new(ApiTestSuite))
}
