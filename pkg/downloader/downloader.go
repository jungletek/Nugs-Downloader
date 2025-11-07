package downloader

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	urlPkg "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/grafov/m3u8"
	"main/pkg/api"
	"main/pkg/config"
	"main/pkg/fsutil"
	"main/pkg/models"
)

// Downloader handles downloading and processing of media content
type Downloader struct {
	apiClient *api.Client
	config    *config.Config
}

// NewDownloader creates a new downloader instance
func NewDownloader(apiClient *api.Client, cfg *config.Config) *Downloader {
	return &Downloader{
		apiClient: apiClient,
		config:    cfg,
	}
}



// DownloadTrack downloads a single track
func (d *Downloader) DownloadTrack(trackPath, url string) error {
	f, err := fsutil.OpenFile(trackPath, os.O_CREATE|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	resp, err := d.apiClient.DownloadFile(url, "https://play.nugs.net/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	totalBytes := resp.ContentLength
	counter := &models.WriteCounter{
		Total:     totalBytes,
		TotalStr:  humanize.Bytes(uint64(totalBytes)),
		StartTime: time.Now().UnixMilli(),
	}

	_, err = io.Copy(f, io.TeeReader(resp.Body, counter))
	fmt.Println("")
	return err
}

// DownloadVideo downloads a video file
func (d *Downloader) DownloadVideo(videoPath, url string) error {
	f, err := fsutil.OpenFile(videoPath, os.O_CREATE|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}
	startByte := stat.Size()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-", startByte))
	httpClient := d.apiClient.GetHTTPClient()
	do, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer do.Body.Close()

	if do.StatusCode != http.StatusOK && do.StatusCode != http.StatusPartialContent {
		return errors.New(do.Status)
	}

	if startByte > 0 {
		fmt.Printf("TS already exists locally, resuming from byte %d...\n", startByte)
		startByte = 0
	}

	totalBytes := do.ContentLength
	counter := &models.WriteCounter{
		Total:     totalBytes,
		TotalStr:  humanize.Bytes(uint64(totalBytes)),
		StartTime: time.Now().UnixMilli(),
		Downloaded: startByte,
	}
	_, err = io.Copy(f, io.TeeReader(do.Body, counter))
	fmt.Println("")
	return err
}

// DownloadLstream downloads livestream segments
func (d *Downloader) DownloadLstream(videoPath string, baseUrl string, segUrls []string) error {
	f, err := fsutil.OpenFile(videoPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	segTotal := len(segUrls)
	for segNum, segUrl := range segUrls {
		segNum++
		fmt.Printf("\rSegment %d of %d.", segNum, segTotal)

		req, err := http.NewRequest(http.MethodGet, baseUrl+segUrl, nil)
		if err != nil {
			return err
		}
		httpClient := d.apiClient.GetHTTPClient()
		do, err := httpClient.Do(req)
		if err != nil {
			return err
		}

		if do.StatusCode != http.StatusOK {
			do.Body.Close()
			return errors.New(do.Status)
		}

		_, err = io.Copy(f, do.Body)
		do.Body.Close()
		if err != nil {
			return err
		}
	}
	fmt.Println("")
	return nil
}

// QueryQuality determines quality from stream URL
func QueryQuality(streamUrl string) *models.Quality {
	for k, v := range models.QualityMap {
		if strings.Contains(streamUrl, k) {
			v.URL = streamUrl
			return &v
		}
	}
	return nil
}

// GetTrackQual selects the best available quality for a track
func GetTrackQual(quals []*models.Quality, wantFmt int) *models.Quality {
	for _, quality := range quals {
		if quality.Format == wantFmt {
			return quality
		}
	}
	return nil
}

// CheckIfHlsOnly checks if all qualities are HLS-only
func CheckIfHlsOnly(quals []*models.Quality) bool {
	for _, quality := range quals {
		if !strings.Contains(quality.URL, ".m3u8?") {
			return false
		}
	}
	return true
}

// ParseHlsMaster parses HLS master playlist
func (d *Downloader) ParseHlsMaster(qual *models.Quality) error {
	master, err := d.apiClient.GetM3U8Playlist(qual.URL)
	if err != nil {
		return err
	}

	sort.Slice(master.Variants, func(x, y int) bool {
		return master.Variants[x].Bandwidth > master.Variants[y].Bandwidth
	})

	variantUri := master.Variants[0].URI
	bitrate := extractBitrate(variantUri)
	if bitrate == "" {
		return errors.New("no regex match for manifest bitrate")
	}

	qual.Specs = bitrate + " Kbps AAC"
	manBase, q, err := d.GetManifestBase(qual.URL)
	if err != nil {
		return err
	}
	qual.URL = manBase + variantUri + q
	return nil
}

// GetManifestBase extracts base URL from manifest URL
func (d *Downloader) GetManifestBase(manifestUrl string) (string, string, error) {
	u, err := urlPkg.Parse(manifestUrl)
	if err != nil {
		return "", "", err
	}
	path := u.Path
	lastPathIdx := strings.LastIndex(path, "/")
	base := u.Scheme + "://" + u.Host + path[:lastPathIdx+1]
	return base, "?" + u.RawQuery, nil
}

// GetSegUrls extracts segment URLs from media playlist
func (d *Downloader) GetSegUrls(manifestUrl, query string) ([]string, error) {
	var segUrls []string
	media, err := d.apiClient.GetMediaPlaylist(manifestUrl)
	if err != nil {
		return nil, err
	}

	for _, seg := range media.Segments {
		if seg == nil {
			break
		}
		segUrls = append(segUrls, seg.URI+query)
	}
	return segUrls, nil
}

// ChooseVariant selects the best video variant
func (d *Downloader) ChooseVariant(manifestUrl, wantRes string) (*m3u8.Variant, string, error) {
	origWantRes := wantRes
	var wantVariant *m3u8.Variant

	master, err := d.apiClient.GetM3U8Playlist(manifestUrl)
	if err != nil {
		return nil, "", err
	}

	sort.Slice(master.Variants, func(x, y int) bool {
		return master.Variants[x].Bandwidth > master.Variants[y].Bandwidth
	})

	if wantRes == "2160" {
		variant := master.Variants[0]
		varRes := strings.SplitN(variant.Resolution, "x", 2)[1]
		varRes = formatRes(varRes)
		return variant, varRes, nil
	}

	for {
		wantVariant = getVidVariant(master.Variants, wantRes)
		if wantVariant != nil {
			break
		} else {
			if fallback, exists := models.ResFallback[wantRes]; exists {
				wantRes = fallback
			} else {
				break
			}
		}
	}

	if wantVariant == nil {
		return nil, "", errors.New("No variant was chosen.")
	}

	if wantRes != origWantRes {
		fmt.Println("Unavailable in your chosen format.")
	}

	wantRes = formatRes(wantRes)
	return wantVariant, wantRes, nil
}

// getVidVariant finds variant by resolution
func getVidVariant(variants []*m3u8.Variant, wantRes string) *m3u8.Variant {
	for _, variant := range variants {
		if strings.HasSuffix(variant.Resolution, "x"+wantRes) {
			return variant
		}
	}
	return nil
}

// formatRes formats resolution for display
func formatRes(res string) string {
	if res == "2160" {
		return "4K"
	} else {
		return res + "p"
	}
}

// extractBitrate extracts bitrate from manifest URL
func extractBitrate(manUrl string) string {
	regex := regexp.MustCompile(`[\w]+(?:_(\d+)k_v\d+)`)
	match := regex.FindStringSubmatch(manUrl)
	if match != nil {
		return match[1]
	}
	return ""
}

// GetKey retrieves encryption key
func GetKey(keyUrl string, apiClient *api.Client) ([]byte, error) {
	httpClient := apiClient.GetHTTPClient()
	resp, err := httpClient.Get(keyUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	buf := make([]byte, 16)
	_, err = io.ReadFull(resp.Body, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// DecryptTrack decrypts AES-encrypted track
func DecryptTrack(key, iv []byte) ([]byte, error) {
	encData, err := os.ReadFile("temp_enc.ts")
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	ecb := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encData))
	fmt.Println("Decrypting...")
	ecb.CryptBlocks(decrypted, encData)
	return decrypted, nil
}

// TsToAac converts TS to AAC using ffmpeg
func TsToAac(decData []byte, outPath, ffmpegNameStr string) error {
	var errBuffer bytes.Buffer
	cmd := exec.Command(ffmpegNameStr, "-i", "pipe:", "-c:a", "copy", outPath)
	cmd.Stdin = bytes.NewReader(decData)
	cmd.Stderr = &errBuffer

	err := cmd.Run()
	if err != nil {
		errString := fmt.Sprintf("%s\n%s", err, errBuffer.String())
		return errors.New(errString)
	}
	return nil
}

// HlsOnly processes HLS-only tracks
func (d *Downloader) HlsOnly(trackPath, manUrl, ffmpegNameStr string) error {
	media, err := d.apiClient.GetMediaPlaylist(manUrl)
	if err != nil {
		return err
	}

	tsUrl := media.Segments[0].URI
	key := media.Key

	// Construct full URLs if they're relative
	manBase, query, err := d.GetManifestBase(manUrl)
	if err != nil {
		return err
	}

	// Construct full segment URL if it's relative
	if !strings.HasPrefix(tsUrl, "http") {
		tsUrl = manBase + tsUrl + query
	}

	// Construct full key URL if it's relative
	keyUrl := key.URI
	if !strings.HasPrefix(keyUrl, "http") {
		keyUrl = manBase + key.URI
	}

	keyBytes, err := GetKey(keyUrl, d.apiClient)
	if err != nil {
		return err
	}

	iv, err := hex.DecodeString(key.IV[2:])
	if err != nil {
		return err
	}

	err = d.DownloadTrack("temp_enc.ts", tsUrl)
	if err != nil {
		return err
	}

	decData, err := DecryptTrack(keyBytes, iv)
	if err != nil {
		return err
	}

	err = os.Remove("temp_enc.ts")
	if err != nil {
		return err
	}

	// Convert to AAC with temporary filename, then tag it
	tempAacPath := trackPath + ".tmp"
	err = TsToAac(decData, tempAacPath, ffmpegNameStr)
	if err != nil {
		return err
	}

	// Tag the AAC file (metadata will be added by the caller)
	// For now, just move the temp file to final location
	err = os.Rename(tempAacPath, trackPath)
	if err != nil {
		os.Remove(tempAacPath) // Clean up on error
		return err
	}

	return nil
}

// HlsOnlyWithMetadata processes HLS-only tracks with metadata tagging
func (d *Downloader) HlsOnlyWithMetadata(trackPath, manUrl, ffmpegNameStr string, metadata *models.TrackMetadata) error {
	media, err := d.apiClient.GetMediaPlaylist(manUrl)
	if err != nil {
		return err
	}

	tsUrl := media.Segments[0].URI
	key := media.Key

	// Construct full URLs if they're relative
	manBase, query, err := d.GetManifestBase(manUrl)
	if err != nil {
		return err
	}

	// Construct full segment URL if it's relative
	if !strings.HasPrefix(tsUrl, "http") {
		tsUrl = manBase + tsUrl + query
	}

	// Construct full key URL if it's relative
	keyUrl := key.URI
	if !strings.HasPrefix(keyUrl, "http") {
		keyUrl = manBase + key.URI
	}

	keyBytes, err := GetKey(keyUrl, d.apiClient)
	if err != nil {
		return err
	}

	iv, err := hex.DecodeString(key.IV[2:])
	if err != nil {
		return err
	}

	err = d.DownloadTrack("temp_enc.ts", tsUrl)
	if err != nil {
		return err
	}

	decData, err := DecryptTrack(keyBytes, iv)
	if err != nil {
		return err
	}

	err = os.Remove("temp_enc.ts")
	if err != nil {
		return err
	}

	// Convert to AAC with temporary filename
	tempAacPath := trackPath + ".tmp"
	err = TsToAac(decData, tempAacPath, ffmpegNameStr)
	if err != nil {
		return err
	}

	// Tag the AAC file with metadata
	err = TagAudioFile(tempAacPath, trackPath, ffmpegNameStr, metadata)
	if err != nil {
		os.Remove(tempAacPath) // Clean up on error
		return err
	}

	// Remove temp file
	err = os.Remove(tempAacPath)
	if err != nil {
		fmt.Printf("Warning: failed to remove temp file %s: %v\n", tempAacPath, err)
	}

	return nil
}

// GetDuration extracts duration from video file using ffmpeg
func GetDuration(tsPath, ffmpegNameStr string) (int, error) {
	var errBuffer bytes.Buffer
	args := []string{"-hide_banner", "-i", tsPath}
	cmd := exec.Command(ffmpegNameStr, args...)
	cmd.Stderr = &errBuffer

	err := cmd.Run()
	if err != nil && err.Error() != "exit status 1" {
		return 0, err
	}

	errStr := errBuffer.String()
	ok := strings.HasSuffix(
		strings.TrimSpace(errStr), "At least one output file must be specified")
	if !ok {
		errString := fmt.Sprintf("%s\n%s", err, errStr)
		return 0, errors.New(errString)
	}

	dur := extractDuration(errStr)
	if dur == "" {
		return 0, errors.New("No regex match.")
	}

	durSecs, err := parseDuration(dur)
	if err != nil {
		return 0, err
	}

	return durSecs, nil
}

// extractDuration extracts duration from ffmpeg output
func extractDuration(errStr string) string {
	regex := regexp.MustCompile(`Duration: ([\d:.]+)`)
	match := regex.FindStringSubmatch(errStr)
	if match != nil {
		return match[1]
	}
	return ""
}

// parseDuration parses duration string to seconds
func parseDuration(dur string) (int, error) {
	dur = strings.Replace(dur, ":", "h", 1)
	dur = strings.Replace(dur, ":", "m", 1)
	dur = strings.Replace(dur, ".", "s", 1)
	dur += "ms"

	d, err := time.ParseDuration(dur)
	if err != nil {
		return 0, err
	}

	rounded := math.Round(d.Seconds())
	return int(rounded), nil
}

// WriteChapsFile writes chapter metadata to file
func WriteChapsFile(chapters []interface{}, dur int) error {
	f, err := fsutil.OpenFile("chapters_nugs_dl_tmp.txt", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(";FFMETADATA1\n")
	if err != nil {
		return err
	}

	chaptersCount := len(chapters)

	var nextChapStart float64

	for i, chapter := range chapters {
		i++
		isLast := i == chaptersCount

		m := chapter.(map[string]interface{})
		start := m["chapterSeconds"].(float64)

		if !isLast {
			nextChapStart = getNextChapStart(chapters, i)
			if nextChapStart <= start {
				continue
			}
		}

		_, err := f.WriteString("\n[CHAPTER]\n")
		if err != nil {
			return err
		}
		_, err = f.WriteString("TIMEBASE=1/1\n")
		if err != nil {
			return err
		}

		startLine := fmt.Sprintf("START=%d\n", int(math.Round(start)))
		_, err = f.WriteString(startLine)
		if err != nil {
			return err
		}

		if isLast {
			endLine := fmt.Sprintf("END=%d\n", dur)
			_, err = f.WriteString(endLine)
			if err != nil {
				return err
			}
		} else {
			endLine := fmt.Sprintf("END=%d\n", int(math.Round(nextChapStart)-1))
			_, err = f.WriteString(endLine)
			if err != nil {
				return err
			}
		}

		_, err = f.WriteString("TITLE=" + m["chaptername"].(string) + "\n")
		if err != nil {
			return err
		}
	}

	return nil
}

// getNextChapStart gets the start time of the next chapter
func getNextChapStart(chapters []interface{}, idx int) float64 {
	for i, chapter := range chapters {
		if i == idx {
			m := chapter.(map[string]interface{})
			return m["chapterSeconds"].(float64)
		}
	}
	return 0
}

// TsToMp4 converts TS to MP4 using ffmpeg
func TsToMp4(VidPathTs, vidPath, ffmpegNameStr string, chapAvail bool) error {
	var (
		errBuffer bytes.Buffer
		args      []string
	)

	if chapAvail {
		args = []string{
			"-hide_banner", "-i", VidPathTs, "-f", "ffmetadata",
			"-i", "chapters_nugs_dl_tmp.txt", "-map_metadata", "1", "-c", "copy", vidPath,
		}
	} else {
		args = []string{"-hide_banner", "-i", VidPathTs, "-c", "copy", vidPath}
	}

	cmd := exec.Command(ffmpegNameStr, args...)
	cmd.Stderr = &errBuffer

	err := cmd.Run()
	if err != nil {
		errString := fmt.Sprintf("%s\n%s", err, errBuffer.String())
		return errors.New(errString)
	}

	return nil
}

// Sanitise sanitizes filename for filesystem
func Sanitise(filename string) string {
	san := regexp.MustCompile(`[\/:*?"><|]`).ReplaceAllString(filename, "_")
	return strings.TrimSuffix(san, "\t")
}

// TagAudioFile adds metadata to audio files using ffmpeg
func TagAudioFile(inputPath, outputPath, ffmpegNameStr string, metadata *models.TrackMetadata) error {
	var args []string

	// Base arguments
	args = append(args, "-hide_banner", "-i", inputPath)

	// Add metadata flags (only if metadata is not nil)
	if metadata != nil {
		if metadata.Title != "" {
			args = append(args, "-metadata", "title="+metadata.Title)
		}
		if metadata.Artist != "" {
			args = append(args, "-metadata", "artist="+metadata.Artist)
		}
		if metadata.Album != "" {
			args = append(args, "-metadata", "album="+metadata.Album)
		}
		if metadata.TrackNum > 0 {
			args = append(args, "-metadata", fmt.Sprintf("track=%d", metadata.TrackNum))
		}
		if metadata.Year != "" {
			args = append(args, "-metadata", "year="+metadata.Year)
		}
	}

	// Copy codecs without re-encoding
	args = append(args, "-c", "copy", outputPath)

	var errBuffer bytes.Buffer
	cmd := exec.Command(ffmpegNameStr, args...)
	cmd.Stderr = &errBuffer

	err := cmd.Run()
	if err != nil {
		errString := fmt.Sprintf("ffmpeg tagging failed: %s\n%s", err, errBuffer.String())
		return errors.New(errString)
	}

	return nil
}

// TagVideoFile adds metadata to video files using ffmpeg
func TagVideoFile(inputPath, outputPath, ffmpegNameStr string, metadata *models.TrackMetadata) error {
	var args []string

	// Base arguments
	args = append(args, "-hide_banner", "-i", inputPath)

	// Add metadata flags (only if metadata is not nil)
	if metadata != nil {
		if metadata.Title != "" {
			args = append(args, "-metadata", "title="+metadata.Title)
		}
		if metadata.Artist != "" {
			args = append(args, "-metadata", "artist="+metadata.Artist)
		}
		if metadata.Album != "" {
			args = append(args, "-metadata", "album="+metadata.Album)
		}
	}

	// Copy codecs without re-encoding
	args = append(args, "-c", "copy", outputPath)

	var errBuffer bytes.Buffer
	cmd := exec.Command(ffmpegNameStr, args...)
	cmd.Stderr = &errBuffer

	err := cmd.Run()
	if err != nil {
		errString := fmt.Sprintf("ffmpeg video tagging failed: %s\n%s", err, errBuffer.String())
		return errors.New(errString)
	}

	return nil
}

// DownloadTrackWithMetadata downloads a track and adds metadata
func (d *Downloader) DownloadTrackWithMetadata(trackPath, url string, metadata *models.TrackMetadata, ffmpegNameStr string) error {
	// Download to temporary file first
	tempPath := trackPath + ".tmp"
	f, err := fsutil.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	resp, err := d.apiClient.DownloadFile(url, "https://play.nugs.net/")
	if err != nil {
		os.Remove(tempPath) // Clean up on error
		return err
	}
	defer resp.Body.Close()

	totalBytes := resp.ContentLength
	counter := &models.WriteCounter{
		Total:     totalBytes,
		TotalStr:  humanize.Bytes(uint64(totalBytes)),
		StartTime: time.Now().UnixMilli(),
	}

	_, err = io.Copy(f, io.TeeReader(resp.Body, counter))
	fmt.Println("")
	if err != nil {
		os.Remove(tempPath) // Clean up on error
		return err
	}

	// Close the temp file before tagging
	f.Close()

	// Tag the file with metadata
	err = TagAudioFile(tempPath, trackPath, ffmpegNameStr, metadata)
	if err != nil {
		os.Remove(tempPath) // Clean up on error
		return err
	}

	// Remove temp file
	err = os.Remove(tempPath)
	if err != nil {
		fmt.Printf("Warning: failed to remove temp file %s: %v\n", tempPath, err)
	}

	return nil
}

// FileExists checks if file exists
func FileExists(path string) (bool, error) {
	f, err := os.Stat(path)
	if err == nil {
		return !f.IsDir(), nil
	} else if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// CheckDiskSpace checks if there's enough disk space for a download
func CheckDiskSpace(path string, requiredBytes int64) error {
	dir := filepath.Dir(path)

	// Cross-platform disk space check - try to create a test file
	testFile := filepath.Join(dir, ".disk_space_test.tmp")
	f, err := os.Create(testFile)
	if err != nil {
		return models.NewDownloadError(models.ErrFileSystem, "Cannot check disk space", "Ensure the download directory is accessible and writable", false, err)
	}
	defer os.Remove(testFile)
	defer f.Close()

	// For large files, we can't easily check exact space, so we'll do a basic check
	// This is a simplified approach that works cross-platform
	if requiredBytes > 100*1024*1024 { // 100MB threshold
		// Try to allocate some space as a basic check
		testSize := 1024 * 1024 // 1MB test
		data := make([]byte, testSize)
		for i := range data {
			data[i] = 0
		}

		if _, err := f.Write(data); err != nil {
			return models.NewDownloadError(models.ErrDiskSpace, "Insufficient disk space for download", "Free up disk space or choose a different download location", false, err)
		}
	}

	return nil
}

// SafeDownloadTrack performs robust track download with error recovery
func (d *Downloader) SafeDownloadTrack(trackPath, url string, expectedSize int64) error {
	// Check disk space first
	if expectedSize > 0 {
		if err := CheckDiskSpace(trackPath, expectedSize); err != nil {
			return err
		}
	}

	// Use temporary file for atomic writes
	tempPath := trackPath + ".tmp"
	defer func() {
		// Clean up temp file if it still exists
		if _, err := os.Stat(tempPath); err == nil {
			os.Remove(tempPath)
		}
	}()

	// Create temp file
	f, err := fsutil.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return models.NewDownloadError(models.ErrFileSystem, "Cannot create temporary file", "Check write permissions for the download directory", false, err)
	}
	defer f.Close()

	// Download with retry logic
	resp, err := d.downloadFileWithRetry(url, "https://play.nugs.net/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Track download progress
	totalBytes := resp.ContentLength
	if totalBytes <= 0 {
		totalBytes = expectedSize
	}

	counter := &models.WriteCounter{
		Total:     totalBytes,
		TotalStr:  humanize.Bytes(uint64(totalBytes)),
		StartTime: time.Now().UnixMilli(),
	}

	// Copy with error handling
	_, err = io.Copy(f, io.TeeReader(resp.Body, counter))
	fmt.Println("")

	if err != nil {
		// Check for specific error types
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return models.NewDownloadError(models.ErrTimeout, "Download timeout", "Check your internet connection and try again", true, err)
		}
		if err == io.ErrUnexpectedEOF {
			return models.NewDownloadError(models.ErrCorruption, "Download incomplete (unexpected EOF)", "The file may be corrupted - try downloading again", true, err)
		}
		return models.NewDownloadError(models.ErrNetwork, "Download failed", "Check your internet connection and try again", true, err)
	}

	// Close file before atomic rename
	f.Close()

	// Validate downloaded file size
	if totalBytes > 0 {
		if stat, err := os.Stat(tempPath); err == nil {
			if stat.Size() != totalBytes {
				os.Remove(tempPath)
				return models.NewDownloadError(models.ErrCorruption, "Downloaded file size mismatch", "The download may be corrupted - try again", true, nil)
			}
		}
	}

	// Atomic rename to final location
	if err := os.Rename(tempPath, trackPath); err != nil {
		os.Remove(tempPath)
		return models.NewDownloadError(models.ErrFileSystem, "Cannot finalize download", "Check write permissions for the download directory", false, err)
	}

	return nil
}

// downloadFileWithRetry downloads a file with retry logic
func (d *Downloader) downloadFileWithRetry(url, referer string) (*http.Response, error) {
	const maxRetries = 3
	const baseDelay = time.Second

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(attempt) * baseDelay
			fmt.Printf("Retrying download in %v... (attempt %d/%d)\n", delay, attempt+1, maxRetries)
			time.Sleep(delay)
		}

		resp, err := d.apiClient.DownloadFile(url, referer)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable
		if netErr, ok := err.(net.Error); ok {
			if !netErr.Timeout() && !netErr.Temporary() {
				// Non-retryable error
				break
			}
		} else if urlErr, ok := err.(*urlPkg.Error); ok {
			if urlErr.Timeout() || urlErr.Temporary() {
				continue // Retry timeouts and temporary errors
			}
		}
	}

	return nil, models.NewDownloadError(models.ErrNetwork, "Download failed after retries", "Check your internet connection and try again later", false, lastErr)
}

// ValidateAudioFile validates downloaded audio file integrity
func ValidateAudioFile(filePath, ffmpegNameStr string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return models.NewDownloadError(models.ErrFileSystem, "Audio file not found", "The file may have been deleted or moved", false, err)
	}

	// Quick FFmpeg probe to check file integrity
	var errBuffer bytes.Buffer
	cmd := exec.Command(ffmpegNameStr, "-hide_banner", "-i", filePath, "-f", "null", "-")
	cmd.Stderr = &errBuffer

	err := cmd.Run()
	if err != nil {
		stderr := errBuffer.String()

		// Parse FFmpeg errors
		if strings.Contains(stderr, "Invalid data found") || strings.Contains(stderr, "corrupt") {
			return models.NewDownloadError(models.ErrCorruption, "Audio file appears corrupted", "Try re-downloading the track", true, err)
		}
		if strings.Contains(stderr, "No such file") {
			return models.NewDownloadError(models.ErrFFmpeg, "FFmpeg not found", "Install FFmpeg and ensure it's in your PATH", false, err)
		}

		return models.NewDownloadError(models.ErrFFmpeg, "Audio file validation failed", "The file may be corrupted or FFmpeg encountered an error", true, err)
	}

	return nil
}

// ParseFFmpegError analyzes FFmpeg error output for actionable feedback
func ParseFFmpegError(err error, stderr string) (models.ErrorType, string, string) {
	if err == nil {
		return models.ErrUnknown, "No error", ""
	}

	// Check for specific FFmpeg error patterns
	if strings.Contains(stderr, "No such file or directory") {
		return models.ErrFFmpeg, "FFmpeg executable not found", "Install FFmpeg and ensure it's in your PATH"
	}

	if strings.Contains(stderr, "Permission denied") {
		return models.ErrFileSystem, "Permission denied accessing file", "Check file permissions and try again"
	}

	if strings.Contains(stderr, "Invalid data found") || strings.Contains(stderr, "corrupt") {
		return models.ErrCorruption, "File appears corrupted", "Try re-downloading the file"
	}

	if strings.Contains(stderr, "No space left on device") {
		return models.ErrDiskSpace, "Disk full during processing", "Free up disk space and try again"
	}

	if strings.Contains(stderr, "Cannot load") || strings.Contains(stderr, "Unsupported codec") {
		return models.ErrCorruption, "Unsupported or corrupted media format", "The source file may be corrupted"
	}

	// Generic FFmpeg error
	return models.ErrFFmpeg, "FFmpeg processing failed", "Check FFmpeg installation and file integrity"
}

// CleanupTempFiles removes temporary files that may be left behind
func CleanupTempFiles(basePath string) error {
	patterns := []string{
		"*.tmp",
		"temp_enc.ts",
		"chapters_nugs_dl_tmp.txt",
		"*_tagged.*",
	}

	var lastErr error
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(filepath.Dir(basePath), pattern))
		if err != nil {
			lastErr = err
			continue
		}

		for _, match := range matches {
			// Only remove files older than 1 hour to avoid deleting active downloads
			if stat, err := os.Stat(match); err == nil {
				if time.Since(stat.ModTime()) > time.Hour {
					os.Remove(match)
				}
			}
		}
	}

	return lastErr
}
