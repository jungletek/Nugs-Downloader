package models

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// TestSuite for models package
type ModelsTestSuite struct {
	suite.Suite
}

// SetupTest runs before each test
func (suite *ModelsTestSuite) SetupTest() {
	// Setup code if needed
}

// TearDownTest runs after each test
func (suite *ModelsTestSuite) TearDownTest() {
	// Cleanup code if needed
}

// TestWriteCounter_Write tests the WriteCounter progress tracking
func (suite *ModelsTestSuite) TestWriteCounter_Write() {
	wc := &WriteCounter{
		Total:     1000,
		TotalStr:  "1000",
		StartTime: time.Now().UnixMilli(),
	}

	// Test writing data
	data := []byte("hello world") // 11 bytes
	n, err := wc.Write(data)

	// Assertions
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 11, n)
	assert.Equal(suite.T(), int64(11), wc.Downloaded)
	assert.True(suite.T(), wc.Percentage >= 1) // Should be at least 1%
}

// TestWriteCounter_Write_ZeroTotal tests edge case with zero total
func (suite *ModelsTestSuite) TestWriteCounter_Write_ZeroTotal() {
	wc := &WriteCounter{
		Total:     0,
		TotalStr:  "0",
		StartTime: time.Now().UnixMilli(),
	}

	data := []byte("test")
	n, err := wc.Write(data)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 4, n)
	assert.Equal(suite.T(), int64(4), wc.Downloaded)
	// Percentage calculation with zero total should not panic
	assert.Equal(suite.T(), 0, wc.Percentage)
}

// TestCheckUrl_Album tests URL pattern matching for albums
func (suite *ModelsTestSuite) TestCheckUrl_Album() {
	url := "https://play.nugs.net/release/12345"
	id, mediaType := CheckUrl(url)

	assert.Equal(suite.T(), "12345", id)
	assert.Equal(suite.T(), 0, mediaType)
}

// TestCheckUrl_Playlist tests URL pattern matching for playlists
func (suite *ModelsTestSuite) TestCheckUrl_Playlist() {
	url := "https://play.nugs.net/#/playlists/playlist/67890"
	id, mediaType := CheckUrl(url)

	assert.Equal(suite.T(), "67890", id)
	assert.Equal(suite.T(), 1, mediaType)
}

// TestCheckUrl_Artist tests URL pattern matching for artists
func (suite *ModelsTestSuite) TestCheckUrl_Artist() {
	// Test old format
	url := "https://play.nugs.net/artist/1125"
	id, mediaType := CheckUrl(url)

	assert.Equal(suite.T(), "1125", id)
	assert.Equal(suite.T(), 5, mediaType)

	// Test new browse format
	url2 := "https://play.nugs.net/browse/artist/1125"
	id2, mediaType2 := CheckUrl(url2)

	assert.Equal(suite.T(), "1125", id2)
	assert.Equal(suite.T(), 5, mediaType2)
}

// TestCheckUrl_Invalid tests invalid URL
func (suite *ModelsTestSuite) TestCheckUrl_Invalid() {
	url := "https://invalid-url.com"
	id, mediaType := CheckUrl(url)

	assert.Equal(suite.T(), "", id)
	assert.Equal(suite.T(), 0, mediaType)
}

// TestGetItemTypeName tests media type name conversion
func (suite *ModelsTestSuite) TestGetItemTypeName() {
	testCases := []struct {
		mediaType int
		expected  string
	}{
		{0, "album"},
		{1, "playlist"},
		{2, "playlist"},
		{3, "catalog_playlist"},
		{4, "video"},
		{5, "artist"},
		{6, "livestream"},
		{7, "livestream"},
		{8, "livestream"},
		{9, "paid_livestream"},
		{10, "video"},
		{99, "unknown"},
	}

	for _, tc := range testCases {
		result := GetItemTypeName(tc.mediaType)
		assert.Equal(suite.T(), tc.expected, result, "Failed for mediaType %d", tc.mediaType)
	}
}

// TestExtractLegToken_Valid tests JWT token extraction
func (suite *ModelsTestSuite) TestExtractLegToken_Valid() {
	// Create a mock JWT payload
	payload := Payload{
		LegacyToken: "test-legacy-token",
		LegacyUguid: "test-uguid",
	}

	payloadBytes, _ := json.Marshal(payload)
	encodedPayload := strings.TrimRight(base64.RawURLEncoding.EncodeToString(payloadBytes), "=")

	// Create full JWT (header.payload.signature)
	token := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9." + encodedPayload + ".signature"

	legacyToken, uguid, err := ExtractLegToken(token)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "test-legacy-token", legacyToken)
	assert.Equal(suite.T(), "test-uguid", uguid)
}

// TestExtractLegToken_InvalidBase64 tests invalid base64
func (suite *ModelsTestSuite) TestExtractLegToken_InvalidBase64() {
	token := "header.invalid-base64.signature"

	_, _, err := ExtractLegToken(token)

	assert.Error(suite.T(), err)
}

// TestExtractLegToken_InvalidJSON tests invalid JSON in payload
func (suite *ModelsTestSuite) TestExtractLegToken_InvalidJSON() {
	// Create invalid JSON payload
	invalidJSON := base64.RawURLEncoding.EncodeToString([]byte("invalid json"))
	token := "header." + invalidJSON + ".signature"

	_, _, err := ExtractLegToken(token)

	assert.Error(suite.T(), err)
}

// TestParseTimestamps tests timestamp parsing
func (suite *ModelsTestSuite) TestParseTimestamps() {
	start := "01/15/2024 10:30:00"
	end := "01/15/2024 11:30:00"

	startStamp, endStamp := ParseTimestamps(start, end)

	// Expected Unix timestamps for the test dates (parsed as UTC)
	expectedStart := "1705314600" // 01/15/2024 10:30:00 UTC
	expectedEnd := "1705318200"   // 01/15/2024 11:30:00 UTC

	assert.Equal(suite.T(), expectedStart, startStamp)
	assert.Equal(suite.T(), expectedEnd, endStamp)
}

// TestParseStreamParams tests stream parameter creation
func (suite *ModelsTestSuite) TestParseStreamParams() {
	subInfo := &SubInfo{
		LegacySubscriptionID: "sub-123",
		Plan: Plan{
			Description: "Premium Plan",
			PlanID:      "plan-456",
		},
		Promo: Promo{
			Plan: Plan{
				Description: "Promo Plan",
				PlanID:      "promo-789",
			},
		},
		StartedAt: "01/01/2024 00:00:00",
		EndsAt:    "12/31/2024 23:59:59",
	}

	// Test regular plan
	params := ParseStreamParams("user-123", subInfo, false)
	assert.Equal(suite.T(), "sub-123", params.SubscriptionID)
	assert.Equal(suite.T(), "plan-456", params.SubCostplanIDAccessList)
	assert.Equal(suite.T(), "user-123", params.UserID)

	// Test promo plan
	paramsPromo := ParseStreamParams("user-123", subInfo, true)
	assert.Equal(suite.T(), "promo-789", paramsPromo.SubCostplanIDAccessList)
}

// TestGetPlan tests plan extraction
func (suite *ModelsTestSuite) TestGetPlan() {
	// Test regular plan
	subInfo := &SubInfo{
		Plan: Plan{
			Description: "Premium Plan",
			PlanID:      "plan-456",
		},
	}

	plan, isPromo := GetPlan(subInfo)
	assert.Equal(suite.T(), "Premium Plan", plan)
	assert.False(suite.T(), isPromo)

	// Test promo plan (when regular plan is zero)
	subInfo.Plan = Plan{} // Zero value
	subInfo.Promo = Promo{
		Plan: Plan{
			Description: "Promo Plan",
			PlanID:      "promo-789",
		},
	}

	plan, isPromo = GetPlan(subInfo)
	assert.Equal(suite.T(), "Promo Plan", plan)
	assert.True(suite.T(), isPromo)
}

// TestQualityMap tests quality mapping constants
func (suite *ModelsTestSuite) TestQualityMap() {
	// Test known quality mappings
	alac := QualityMap[".alac16/"]
	assert.Equal(suite.T(), "16-bit / 44.1 kHz ALAC", alac.Specs)
	assert.Equal(suite.T(), ".m4a", alac.Extension)
	assert.Equal(suite.T(), 1, alac.Format)

	flac := QualityMap[".flac16/"]
	assert.Equal(suite.T(), "16-bit / 44.1 kHz FLAC", flac.Specs)
	assert.Equal(suite.T(), ".flac", flac.Extension)
	assert.Equal(suite.T(), 2, flac.Format)
}

// TestResolveRes tests resolution mapping
func (suite *ModelsTestSuite) TestResolveRes() {
	assert.Equal(suite.T(), "480", ResolveRes[1])
	assert.Equal(suite.T(), "720", ResolveRes[2])
	assert.Equal(suite.T(), "1080", ResolveRes[3])
	assert.Equal(suite.T(), "1440", ResolveRes[4])
	assert.Equal(suite.T(), "2160", ResolveRes[5])
}

// TestTrackFallback tests track fallback mapping
func (suite *ModelsTestSuite) TestTrackFallback() {
	assert.Equal(suite.T(), 2, TrackFallback[1])
	assert.Equal(suite.T(), 5, TrackFallback[2])
	assert.Equal(suite.T(), 2, TrackFallback[3])
	assert.Equal(suite.T(), 3, TrackFallback[4])
}

// TestResFallback tests resolution fallback mapping
func (suite *ModelsTestSuite) TestResFallback() {
	assert.Equal(suite.T(), "480", ResFallback["720"])
	assert.Equal(suite.T(), "720", ResFallback["1080"])
	assert.Equal(suite.T(), "1080", ResFallback["1440"])
}

// TestRegexStrings tests URL regex patterns compilation
func (suite *ModelsTestSuite) TestRegexStrings() {
	// Ensure all regex patterns compile without error
	for i, regexStr := range RegexStrings {
		regex, err := regexp.Compile(regexStr)
		assert.NoError(suite.T(), err, "Regex pattern %d failed to compile: %s", i, regexStr)
		assert.NotNil(suite.T(), regex)
	}
}

// TestAuthResponse_JSON tests JSON marshaling/unmarshaling
func (suite *ModelsTestSuite) TestAuthResponse_JSON() {
	auth := AuthResponse{AccessToken: "test-token"}

	data, err := json.Marshal(auth)
	assert.NoError(suite.T(), err)

	var decoded AuthResponse
	err = json.Unmarshal(data, &decoded)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), auth.AccessToken, decoded.AccessToken)
}

// TestUserInfo_JSON tests JSON marshaling/unmarshaling
func (suite *ModelsTestSuite) TestUserInfo_JSON() {
	user := UserInfo{Sub: "user-123"}

	data, err := json.Marshal(user)
	assert.NoError(suite.T(), err)

	var decoded UserInfo
	err = json.Unmarshal(data, &decoded)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), user.Sub, decoded.Sub)
}

// TestTrackMetadata tests TrackMetadata struct
func (suite *ModelsTestSuite) TestTrackMetadata() {
	metadata := TrackMetadata{
		Title:    "Test Song",
		Artist:   "Test Artist",
		Album:    "Test Album",
		TrackNum: 5,
		Year:     "2024",
	}

	assert.Equal(suite.T(), "Test Song", metadata.Title)
	assert.Equal(suite.T(), "Test Artist", metadata.Artist)
	assert.Equal(suite.T(), "Test Album", metadata.Album)
	assert.Equal(suite.T(), 5, metadata.TrackNum)
	assert.Equal(suite.T(), "2024", metadata.Year)
}

// TestTrackMetadata_Empty tests TrackMetadata with empty fields
func (suite *ModelsTestSuite) TestTrackMetadata_Empty() {
	metadata := TrackMetadata{}

	assert.Equal(suite.T(), "", metadata.Title)
	assert.Equal(suite.T(), "", metadata.Artist)
	assert.Equal(suite.T(), "", metadata.Album)
	assert.Equal(suite.T(), 0, metadata.TrackNum)
	assert.Equal(suite.T(), "", metadata.Year)
}

// Run the test suite
func TestModelsTestSuite(t *testing.T) {
	suite.Run(t, new(ModelsTestSuite))
}
