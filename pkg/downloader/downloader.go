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
	"net/http"
	"net/url"
	"os"
	"os/exec"
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
	u, err := url.Parse(manifestUrl)
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

	err = TsToAac(decData, trackPath, ffmpegNameStr)
	return err
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
