package downloader

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResumeState represents the state of a resumable download
type ResumeState struct {
	FilePath       string         `json:"file_path"`
	URL            string         `json:"url"`
	TotalSize      int64          `json:"total_size"`
	DownloadedSize int64          `json:"downloaded_size"`
	LastModified   time.Time      `json:"last_modified"`
	ETag           string         `json:"etag"`
	Checksum       string         `json:"checksum"`
	Segments       []SegmentState `json:"segments,omitempty"` // For livestreams
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// SegmentState tracks individual segment download state for livestreams
type SegmentState struct {
	Index     int    `json:"index"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Checksum  string `json:"checksum"`
	Completed bool   `json:"completed"`
}

// ResumeManager handles download resume state persistence and validation
type ResumeManager struct {
	stateDir string
}

// NewResumeManager creates a new resume manager
func NewResumeManager(stateDir string) *ResumeManager {
	return &ResumeManager{
		stateDir: stateDir,
	}
}

// getStateFilePath returns the path for a resume state file
func (rm *ResumeManager) getStateFilePath(filePath string) string {
	// Create a hash of the file path for the state file name
	hash := md5.Sum([]byte(filePath))
	return filepath.Join(rm.stateDir, hex.EncodeToString(hash[:])+".resume.json")
}

// SaveState saves the resume state to disk
func (rm *ResumeManager) SaveState(state *ResumeState) error {
	// Ensure state directory exists
	if err := os.MkdirAll(rm.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	state.UpdatedAt = time.Now()
	stateFile := rm.getStateFilePath(state.FilePath)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal resume state: %w", err)
	}

	// Write to temporary file first for atomicity
	tempFile := stateFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write resume state: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, stateFile); err != nil {
		os.Remove(tempFile) // Clean up on error
		return fmt.Errorf("failed to save resume state: %w", err)
	}

	return nil
}

// LoadState loads the resume state from disk
func (rm *ResumeManager) LoadState(filePath string) (*ResumeState, error) {
	stateFile := rm.getStateFilePath(filePath)

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No resume state exists
		}
		return nil, fmt.Errorf("failed to read resume state: %w", err)
	}

	var state ResumeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resume state: %w", err)
	}

	return &state, nil
}

// DeleteState removes the resume state file
func (rm *ResumeManager) DeleteState(filePath string) error {
	stateFile := rm.getStateFilePath(filePath)
	if err := os.Remove(stateFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete resume state: %w", err)
	}
	return nil
}

// ValidatePartialDownload checks if a partial download can be resumed
func (rm *ResumeManager) ValidatePartialDownload(state *ResumeState) error {
	// Check if file exists
	stat, err := os.Stat(state.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("partial file no longer exists")
		}
		return fmt.Errorf("failed to stat partial file: %w", err)
	}

	// Check file size matches expected downloaded size
	if stat.Size() != state.DownloadedSize {
		return fmt.Errorf("file size mismatch: expected %d, got %d", state.DownloadedSize, stat.Size())
	}

	// Check if file is too old (resume state older than 24 hours)
	if time.Since(state.UpdatedAt) > 24*time.Hour {
		return fmt.Errorf("resume state is too old")
	}

	// For basic validation, file size match is sufficient
	// More advanced validation could include checksum verification
	return nil
}

// CreateInitialState creates a new resume state for a download
func (rm *ResumeManager) CreateInitialState(filePath, url string, totalSize int64, etag string) *ResumeState {
	return &ResumeState{
		FilePath:       filePath,
		URL:            url,
		TotalSize:      totalSize,
		DownloadedSize: 0,
		LastModified:   time.Now(),
		ETag:           etag,
		Checksum:       "",
		Segments:       nil,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// UpdateProgress updates the downloaded size in the resume state
func (rm *ResumeManager) UpdateProgress(state *ResumeState, downloadedSize int64) error {
	state.DownloadedSize = downloadedSize
	state.UpdatedAt = time.Now()
	return rm.SaveState(state)
}

// SendRangeRequest sends an HTTP request with Range header for resuming downloads
func SendRangeRequest(client *http.Client, url string, startByte int64, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Add Range header
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))

	// Add any additional headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// Check if server supports range requests
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server returned status %d (range requests may not be supported)", resp.StatusCode)
	}

	return resp, nil
}

// CalculateChecksum calculates MD5 checksum of a file (for integrity validation)
func CalculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// CleanupOldStates removes resume state files older than the specified duration
func (rm *ResumeManager) CleanupOldStates(maxAge time.Duration) error {
	entries, err := os.ReadDir(rm.stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist, nothing to clean
		}
		return err
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".resume.json") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > maxAge {
			os.Remove(filepath.Join(rm.stateDir, entry.Name()))
		}
	}

	return nil
}
