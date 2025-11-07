package downloader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ResumeManagerTestSuite struct {
	suite.Suite
	tempDir   string
	resumeMgr *ResumeManager
	testFile  string
}

func (suite *ResumeManagerTestSuite) SetupTest() {
	// Create temporary directory for tests
	tempDir, err := os.MkdirTemp("", "resume_test_*")
	suite.Require().NoError(err)
	suite.tempDir = tempDir

	// Create resume manager
	suite.resumeMgr = NewResumeManager(filepath.Join(tempDir, "resume"))

	// Test file path
	suite.testFile = "/tmp/test_download.mp3"
}

func (suite *ResumeManagerTestSuite) TearDownTest() {
	// Clean up temporary directory
	os.RemoveAll(suite.tempDir)
}

func (suite *ResumeManagerTestSuite) TestCreateInitialState() {
	state := suite.resumeMgr.CreateInitialState(suite.testFile, "http://example.com/file.mp3", 1000, "etag123")

	assert.Equal(suite.T(), suite.testFile, state.FilePath)
	assert.Equal(suite.T(), "http://example.com/file.mp3", state.URL)
	assert.Equal(suite.T(), int64(1000), state.TotalSize)
	assert.Equal(suite.T(), int64(0), state.DownloadedSize)
	assert.Equal(suite.T(), "etag123", state.ETag)
	assert.Empty(suite.T(), state.Checksum)
	assert.Nil(suite.T(), state.Segments)
	assert.False(suite.T(), state.CreatedAt.IsZero())
	assert.False(suite.T(), state.UpdatedAt.IsZero())
}

func (suite *ResumeManagerTestSuite) TestSaveAndLoadState() {
	// Create and save state
	originalState := suite.resumeMgr.CreateInitialState(suite.testFile, "http://example.com/file.mp3", 1000, "etag123")
	originalState.DownloadedSize = 500
	originalState.Checksum = "abc123"

	err := suite.resumeMgr.SaveState(originalState)
	suite.NoError(err)

	// Load state
	loadedState, err := suite.resumeMgr.LoadState(suite.testFile)
	suite.NoError(err)
	suite.NotNil(loadedState)

	// Verify loaded state
	assert.Equal(suite.T(), originalState.FilePath, loadedState.FilePath)
	assert.Equal(suite.T(), originalState.URL, loadedState.URL)
	assert.Equal(suite.T(), originalState.TotalSize, loadedState.TotalSize)
	assert.Equal(suite.T(), originalState.DownloadedSize, loadedState.DownloadedSize)
	assert.Equal(suite.T(), originalState.ETag, loadedState.ETag)
	assert.Equal(suite.T(), originalState.Checksum, loadedState.Checksum)
}

func (suite *ResumeManagerTestSuite) TestLoadStateNonexistent() {
	state, err := suite.resumeMgr.LoadState("/nonexistent/file.mp3")
	suite.NoError(err)
	suite.Nil(state)
}

func (suite *ResumeManagerTestSuite) TestDeleteState() {
	// Create and save state
	state := suite.resumeMgr.CreateInitialState(suite.testFile, "http://example.com/file.mp3", 1000, "etag123")
	err := suite.resumeMgr.SaveState(state)
	suite.NoError(err)

	// Verify it exists
	loadedState, err := suite.resumeMgr.LoadState(suite.testFile)
	suite.NoError(err)
	suite.NotNil(loadedState)

	// Delete state
	err = suite.resumeMgr.DeleteState(suite.testFile)
	suite.NoError(err)

	// Verify it's gone
	loadedState, err = suite.resumeMgr.LoadState(suite.testFile)
	suite.NoError(err)
	suite.Nil(loadedState)
}

func (suite *ResumeManagerTestSuite) TestValidatePartialDownloadValid() {
	// Create a temporary file with expected size
	tempFile := filepath.Join(suite.tempDir, "test_partial.mp3")
	err := os.WriteFile(tempFile, make([]byte, 500), 0644)
	suite.NoError(err)

	// Create resume state
	state := suite.resumeMgr.CreateInitialState(tempFile, "http://example.com/file.mp3", 1000, "etag123")
	state.DownloadedSize = 500

	// Validate
	err = suite.resumeMgr.ValidatePartialDownload(state)
	suite.NoError(err)
}

func (suite *ResumeManagerTestSuite) TestValidatePartialDownloadFileMissing() {
	state := suite.resumeMgr.CreateInitialState("/nonexistent/file.mp3", "http://example.com/file.mp3", 1000, "etag123")
	state.DownloadedSize = 500

	err := suite.resumeMgr.ValidatePartialDownload(state)
	suite.Error(err)
	assert.Contains(suite.T(), err.Error(), "partial file no longer exists")
}

func (suite *ResumeManagerTestSuite) TestValidatePartialDownloadSizeMismatch() {
	// Create a temporary file with wrong size
	tempFile := filepath.Join(suite.tempDir, "test_partial.mp3")
	err := os.WriteFile(tempFile, make([]byte, 400), 0644) // Wrong size
	suite.NoError(err)

	// Create resume state expecting different size
	state := suite.resumeMgr.CreateInitialState(tempFile, "http://example.com/file.mp3", 1000, "etag123")
	state.DownloadedSize = 500

	// Validate
	err = suite.resumeMgr.ValidatePartialDownload(state)
	suite.Error(err)
	assert.Contains(suite.T(), err.Error(), "file size mismatch")
}

func (suite *ResumeManagerTestSuite) TestValidatePartialDownloadTooOld() {
	// Create a temporary file
	tempFile := filepath.Join(suite.tempDir, "test_partial.mp3")
	err := os.WriteFile(tempFile, make([]byte, 500), 0644)
	suite.NoError(err)

	// Create resume state that's too old
	state := suite.resumeMgr.CreateInitialState(tempFile, "http://example.com/file.mp3", 1000, "etag123")
	state.DownloadedSize = 500
	state.UpdatedAt = time.Now().Add(-25 * time.Hour) // 25 hours ago

	// Validate
	err = suite.resumeMgr.ValidatePartialDownload(state)
	suite.Error(err)
	assert.Contains(suite.T(), err.Error(), "resume state is too old")
}

func (suite *ResumeManagerTestSuite) TestUpdateProgress() {
	// Create and save initial state
	state := suite.resumeMgr.CreateInitialState(suite.testFile, "http://example.com/file.mp3", 1000, "etag123")
	err := suite.resumeMgr.SaveState(state)
	suite.NoError(err)

	// Update progress
	err = suite.resumeMgr.UpdateProgress(state, 750)
	suite.NoError(err)
	assert.Equal(suite.T(), int64(750), state.DownloadedSize)

	// Load and verify
	loadedState, err := suite.resumeMgr.LoadState(suite.testFile)
	suite.NoError(err)
	suite.NotNil(loadedState)
	assert.Equal(suite.T(), int64(750), loadedState.DownloadedSize)
}

func (suite *ResumeManagerTestSuite) TestCleanupOldStates() {
	// Create states with different ages
	oldState := suite.resumeMgr.CreateInitialState("/tmp/old.mp3", "http://example.com/old.mp3", 1000, "etag123")
	err := suite.resumeMgr.SaveState(oldState)
	suite.NoError(err)

	// Manually modify the old state file to have an old UpdatedAt
	oldStateFile := suite.resumeMgr.getStateFilePath("/tmp/old.mp3")
	data, err := os.ReadFile(oldStateFile)
	suite.NoError(err)

	var state ResumeState
	err = json.Unmarshal(data, &state)
	suite.NoError(err)

	state.UpdatedAt = time.Now().Add(-48 * time.Hour) // 2 days ago
	data, err = json.MarshalIndent(state, "", "  ")
	suite.NoError(err)

	err = os.WriteFile(oldStateFile, data, 0644)
	suite.NoError(err)

	newState := suite.resumeMgr.CreateInitialState("/tmp/new.mp3", "http://example.com/new.mp3", 1000, "etag456")
	// newState.UpdatedAt is current time
	err = suite.resumeMgr.SaveState(newState)
	suite.NoError(err)

	// Cleanup old states (24 hours threshold)
	err = suite.resumeMgr.CleanupOldStates(24 * time.Hour)
	suite.NoError(err)

	// Old state should be gone
	oldLoaded, err := suite.resumeMgr.LoadState("/tmp/old.mp3")
	suite.NoError(err)
	suite.Nil(oldLoaded)

	// New state should remain
	newLoaded, err := suite.resumeMgr.LoadState("/tmp/new.mp3")
	suite.NoError(err)
	suite.NotNil(newLoaded)
}

func (suite *ResumeManagerTestSuite) TestCalculateChecksumFromBytes() {
	data := []byte("test data for checksum")
	checksum := CalculateChecksumFromBytes(data)

	// MD5 of "test data for checksum" should be consistent
	expected := "a16de13eaa4650a7827e619b6db9fcb7"
	assert.Equal(suite.T(), expected, checksum)
}

// Run the test suite
func TestResumeManagerTestSuite(t *testing.T) {
	suite.Run(t, new(ResumeManagerTestSuite))
}
