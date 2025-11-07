package processor

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"main/pkg/api"
	"main/pkg/config"
	"main/pkg/downloader"
	"main/pkg/fsutil"
	"main/pkg/logger"
	"main/pkg/models"
)

const (
	MaxFolderNameLen   = 100
	MaxVideoFilenameLen = 200
)

var (
	streamMetaIndices = [4]int{1, 4, 7, 10}
)

// Processor handles content processing and downloading
type Processor struct {
	apiClient  *api.Client
	downloader *downloader.Downloader
	config     *config.Config
}

// NewProcessor creates a new processor instance
func NewProcessor(apiClient *api.Client, dl *downloader.Downloader, cfg *config.Config) *Processor {
	return &Processor{
		apiClient:  apiClient,
		downloader: dl,
		config:     cfg,
	}
}

// ProcessAlbum processes an album with graceful error handling
func (p *Processor) ProcessAlbum(albumID string, streamParams *models.StreamParams, artResp *models.AlbArtResp) error {
	var (
		meta   *models.AlbArtResp
		tracks []models.Track
	)

	if albumID == "" {
		meta = artResp
		tracks = meta.Songs
	} else {
		_meta, err := p.apiClient.GetAlbumMeta(albumID)
		if err != nil {
			logger.GetLogger().WithError(err).WithField("album_id", albumID).Error("Failed to get album metadata")
			return models.NewDownloadError(models.ErrNetwork, "Failed to get album metadata", "Check your internet connection and try again", true, err)
		}
		meta = _meta.Response
		tracks = meta.Tracks
	}

	trackTotal := len(tracks)
	skuID := getVideoSku(meta.Products)

	if skuID == 0 && trackTotal < 1 {
		return models.NewDownloadError(models.ErrUnknown, "Release has no tracks or videos", "This release may not be available for download", false, nil)
	}

	if skuID != 0 {
		if p.config.SkipVideos {
			fmt.Println("Video-only album, skipped.")
			return nil
		}
		if p.config.ForceVideo || trackTotal < 1 {
			return p.ProcessVideo(albumID, "", streamParams, meta, false)
		}
	}

	albumFolder := meta.ArtistName + " - " + strings.TrimRight(meta.ContainerInfo, " ")
	fmt.Println(albumFolder)

	if len(albumFolder) > MaxFolderNameLen {
		albumFolder = albumFolder[:MaxFolderNameLen]
		fmt.Printf("Album folder name was chopped because it exceeds %d characters.", MaxFolderNameLen)
	}

	albumPath := filepath.Join(p.config.OutPath, downloader.Sanitise(albumFolder))
	err := fsutil.MakeDirs(albumPath)
	if err != nil {
		return models.NewDownloadError(models.ErrFileSystem, "Failed to create album folder", "Check write permissions for the download directory", false, err)
	}

	// Clean up any leftover temp files from previous runs
	downloader.CleanupTempFiles(albumPath)

	// Track download results for summary
	var successCount, failureCount int
	var failures []string

	for trackNum, track := range tracks {
		trackNum++
		fmt.Printf("Processing track %d of %d: %s\n", trackNum, trackTotal, track.SongTitle)

		err := p.ProcessTrackWithMetadata(albumPath, trackNum, trackTotal, &track, streamParams, meta)
		if err != nil {
			failureCount++
			failureMsg := fmt.Sprintf("Track %d (%s): %v", trackNum, track.SongTitle, err)
			failures = append(failures, failureMsg)

			// Log the error with context
			context := map[string]interface{}{
				"album":     meta.ArtistName + " - " + meta.ContainerInfo,
				"track":     track.SongTitle,
				"track_num": trackNum,
				"total":     trackTotal,
			}
			logger.WrapError(err, context)
			logger.GetLogger().Error("Track download failed", "track", track.SongTitle, "album", meta.ContainerInfo)

			// Check if error is retryable
			if dlErr, ok := err.(*models.DownloadError); ok && dlErr.Retryable {
				fmt.Printf("WARNING: Track %d failed (retryable): %s\n", trackNum, dlErr.UserGuide)
			} else {
				fmt.Printf("ERROR: Track %d failed: %s\n", trackNum, err.Error())
			}
		} else {
			successCount++
			fmt.Printf("SUCCESS: Track %d completed: %s\n", trackNum, track.SongTitle)
		}
	}

	// Provide summary
	fmt.Printf("\nAlbum download summary: %d/%d tracks successful\n", successCount, trackTotal)

	if failureCount > 0 {
		fmt.Printf("%d tracks failed:\n", failureCount)
		for _, failure := range failures {
			fmt.Printf("   - %s\n", failure)
		}

		if successCount > 0 {
			fmt.Println("Partial download completed. Failed tracks can be retried individually.")
			return nil // Don't fail the entire album if some tracks succeeded
		} else {
			return models.NewDownloadError(models.ErrUnknown, "All tracks failed to download", "Check your internet connection and try again", true, nil)
		}
	}

	fmt.Println("Album download completed successfully!")
	return nil
}

// ProcessArtist processes an artist discography
func (p *Processor) ProcessArtist(artistId string, streamParams *models.StreamParams) error {
	meta, err := p.apiClient.GetArtistMeta(artistId)
	if err != nil {
		logger.GetLogger().WithError(err).WithField("artist_id", artistId).Error("Failed to get artist metadata")
		return err
	}

	if len(meta) == 0 {
		return fmt.Errorf("the API didn't return any artist metadata")
	}

	fmt.Println(meta[0].Response.Containers[0].ArtistName)
	albumTotal := getAlbumTotal(meta)

	for _, _meta := range meta {
		for albumNum, container := range _meta.Response.Containers {
			fmt.Printf("Item %d of %d:\n", albumNum+1, albumTotal)
			if p.config.SkipVideos {
				err = p.ProcessAlbum("", streamParams, container)
			} else {
				// Can't re-use this metadata as it doesn't have any product info for videos.
				err = p.ProcessAlbum(strconv.Itoa(container.ContainerID), streamParams, nil)
			}
			if err != nil {
				context := map[string]interface{}{
					"item_type": "artist",
					"artist_id": artistId,
					"item_num":  albumNum + 1,
					"total":     albumTotal,
				}
				logger.WrapError(err, context)
				logger.GetLogger().Error("Artist item failed", "item", albumNum+1, "total", albumTotal)
			}
		}
	}

	return nil
}

// ProcessPlaylist processes a playlist
func (p *Processor) ProcessPlaylist(plistId, legacyToken string, streamParams *models.StreamParams, cat bool) error {
	_meta, err := p.apiClient.GetPlistMeta(plistId, p.config.Email, legacyToken, cat)
	if err != nil {
		logger.GetLogger().WithError(err).WithField("playlist_id", plistId).Error("Failed to get playlist metadata")
		return err
	}

	meta := _meta.Response
	plistName := meta.PlayListName
	fmt.Println(plistName)

	if len(plistName) > MaxFolderNameLen {
		plistName = plistName[:MaxFolderNameLen]
		fmt.Printf("Playlist folder name was chopped because it exceeds %d characters.", MaxFolderNameLen)
	}

	plistPath := filepath.Join(p.config.OutPath, downloader.Sanitise(plistName))
	err = fsutil.MakeDirs(plistPath)
	if err != nil {
		fmt.Println("Failed to make playlist folder.")
		return err
	}

	trackTotal := len(meta.Items)
	for trackNum, track := range meta.Items {
		trackNum++
		err := p.ProcessTrack(plistPath, trackNum, trackTotal, &track.Track, streamParams)
		if err != nil {
			context := map[string]interface{}{
				"playlist":  meta.PlayListName,
				"track":     track.Track.SongTitle,
				"track_num": trackNum,
				"total":     trackTotal,
			}
			logger.WrapError(err, context)
			logger.GetLogger().Error("Playlist track download failed", "track", track.Track.SongTitle, "playlist", meta.PlayListName)
		}
	}

	return nil
}

// ProcessVideo processes a video
func (p *Processor) ProcessVideo(videoID, uguID string, streamParams *models.StreamParams, _meta *models.AlbArtResp, isLstream bool) error {
	var (
		chapsAvail bool
		skuID      int
		manifestUrl string
		meta       *models.AlbArtResp
		err        error
	)

	if _meta != nil {
		meta = _meta
	} else {
		m, err := p.apiClient.GetAlbumMeta(videoID)
		if err != nil {
			logger.GetLogger().WithError(err).WithField("video_id", videoID).Error("Failed to get video metadata")
			return err
		}
		meta = m.Response
	}

	if !p.config.SkipChapters {
		chapsAvail = !reflect.ValueOf(meta.VideoChapters).IsZero()
	}

	videoFname := meta.ArtistName + " - " + strings.TrimRight(meta.ContainerInfo, " ")
	fmt.Println(videoFname)

	if len(videoFname) > MaxVideoFilenameLen {
		videoFname = videoFname[:MaxVideoFilenameLen]
		fmt.Printf("Video filename was chopped because it exceeds %d characters.", MaxVideoFilenameLen)
	}

	if isLstream {
		skuID = getLstreamSku(meta.ProductFormatList)
	} else {
		skuID = getVideoSku(meta.Products)
	}

	if skuID == 0 {
		return fmt.Errorf("no video available")
	}

	if uguID == "" {
		manifestUrl, err = p.apiClient.GetStreamMeta(meta.ContainerID, skuID, 0, streamParams)
	} else {
		manifestUrl, err = p.apiClient.GetPurchasedManUrl(skuID, videoID, streamParams.UserID, uguID)
	}

	if err != nil {
		fmt.Println("Failed to get video file metadata.")
		return err
	} else if manifestUrl == "" {
		return fmt.Errorf("the api didn't return a video manifest url")
	}

	variant, retRes, err := p.downloader.ChooseVariant(manifestUrl, p.config.WantRes)
	if err != nil {
		fmt.Println("Failed to get video master manifest.")
		return err
	}

	vidPathNoExt := filepath.Join(p.config.OutPath, downloader.Sanitise(videoFname+"_"+retRes))
	VidPathTs := vidPathNoExt + ".ts"
	vidPath := vidPathNoExt + ".mp4"

	exists, err := downloader.FileExists(vidPath)
	if err != nil {
		fmt.Println("Failed to check if video already exists locally.")
		return err
	}

	if exists {
		fmt.Println("Video already exists locally.")
		return nil
	}

	manBaseUrl, query, err := p.downloader.GetManifestBase(manifestUrl)
	if err != nil {
		fmt.Println("Failed to get video manifest base URL.")
		return err
	}

	segUrls, err := p.downloader.GetSegUrls(manBaseUrl+variant.URI, query)
	if err != nil {
		fmt.Println("Failed to get video segment URLs.")
		return err
	}

	// Player album page videos aren't always only the first seg for the entire vid.
	isLstream = segUrls[0] != segUrls[1]

	if !isLstream {
		fmt.Printf("%.3f FPS, ", variant.FrameRate)
	}
	fmt.Printf("%d Kbps, %s (%s)\n", variant.Bandwidth/1000, retRes, variant.Resolution)

	if isLstream {
		err = p.downloader.DownloadLstream(VidPathTs, manBaseUrl, segUrls)
	} else {
		err = p.downloader.DownloadVideo(VidPathTs, manBaseUrl+segUrls[0])
	}

	if err != nil {
		fmt.Println("Failed to download video segments.")
		return err
	}

	if chapsAvail {
		dur, err := downloader.GetDuration(VidPathTs, p.config.FfmpegNameStr)
		if err != nil {
			fmt.Println("Failed to get TS duration.")
			return err
		}
		err = downloader.WriteChapsFile(meta.VideoChapters, dur)
		if err != nil {
			fmt.Println("Failed to write chapters file.")
			return err
		}
	}

	fmt.Println("Putting into MP4 container...")
	err = downloader.TsToMp4(VidPathTs, vidPath, p.config.FfmpegNameStr, chapsAvail)
	if err != nil {
		fmt.Println("Failed to put TS into MP4 container.")
		return err
	}

	if chapsAvail {
		err = os.Remove("chapters_nugs_dl_tmp.txt")
		if err != nil {
			fmt.Println("Failed to delete chapters file.")
		}
	}

	err = os.Remove(VidPathTs)
	if err != nil {
		fmt.Println("Failed to delete TS.")
	}

	return nil
}

// ProcessTrack processes a single track
func (p *Processor) ProcessTrack(folPath string, trackNum, trackTotal int, track *models.Track, streamParams *models.StreamParams) error {
	return p.ProcessTrackWithMetadata(folPath, trackNum, trackTotal, track, streamParams, nil)
}

// ProcessTrackWithMetadata processes a single track with metadata
func (p *Processor) ProcessTrackWithMetadata(folPath string, trackNum, trackTotal int, track *models.Track, streamParams *models.StreamParams, albumMeta *models.AlbArtResp) error {
	origWantFmt := p.config.Format
	wantFmt := origWantFmt
	var (
		quals     []*models.Quality
		chosenQual *models.Quality
	)

	// Call the stream meta endpoint four times to get all avail formats since the formats can shift.
	// This will ensure the right format's always chosen.
	for _, i := range streamMetaIndices {
		streamUrl, err := p.apiClient.GetStreamMeta(track.TrackID, 0, i, streamParams)
		if err != nil {
			logger.GetLogger().Error("Failed to get track stream metadata", "error", err, "track_id", track.TrackID)
			return err
		} else if streamUrl == "" {
			return fmt.Errorf("the api didn't return a track stream URL")
		}

		quality := downloader.QueryQuality(streamUrl)
		if quality == nil {
			logger.GetLogger().Warn("API returned unsupported format", "url", streamUrl, "track_id", track.TrackID)
			continue
		}
		quals = append(quals, quality)
	}

	if len(quals) == 0 {
		return fmt.Errorf("the api didn't return any formats")
	}

	isHlsOnly := downloader.CheckIfHlsOnly(quals)

	// Create metadata for the track
	var metadata *models.TrackMetadata
	if albumMeta != nil {
		metadata = &models.TrackMetadata{
			Title:    track.SongTitle,
			Artist:   albumMeta.ArtistName,
			Album:    albumMeta.ContainerInfo,
			TrackNum: trackNum,
		}
	}

	if isHlsOnly {
		fmt.Println("HLS-only track. Only AAC is available.")
		chosenQual = quals[0]
		err := p.downloader.ParseHlsMaster(chosenQual)
		if err != nil {
			return err
		}
	} else {
		for {
			chosenQual = downloader.GetTrackQual(quals, wantFmt)
			if chosenQual != nil {
				break
			} else {
				// Fallback quality.
				wantFmt = models.TrackFallback[wantFmt]
			}
		}
		if chosenQual == nil {
			return fmt.Errorf("no track format was chosen")
		}
		if wantFmt != origWantFmt && origWantFmt != 4 {
			fmt.Println("Unavailable in your chosen format.")
		}
	}

	trackFname := fmt.Sprintf("%02d. %s%s", trackNum, downloader.Sanitise(track.SongTitle), chosenQual.Extension)
	trackPath := filepath.Join(folPath, trackFname)

	exists, err := downloader.FileExists(trackPath)
	if err != nil {
		fmt.Println("Failed to check if track already exists locally.")
		return err
	}

	if exists {
		fmt.Println("Track already exists locally.")
		return nil
	}

	fmt.Printf("Downloading track %d of %d: %s - %s\n", trackNum, trackTotal, track.SongTitle, chosenQual.Specs)

	if isHlsOnly {
		if metadata != nil {
			err = p.processHlsOnlyWithMetadata(trackPath, chosenQual.URL, metadata)
		} else {
			err = p.processHlsOnly(trackPath, chosenQual.URL)
		}
	} else {
		if metadata != nil {
			err = p.processTrackWithMetadata(trackPath, chosenQual.URL, metadata)
		} else {
			err = p.processTrack(trackPath, chosenQual.URL)
		}
	}

	if err != nil {
		// Provide user-friendly error messages
		if dlErr, ok := err.(*models.DownloadError); ok {
			return dlErr // Already structured error
		}
		return models.NewDownloadError(models.ErrUnknown, "Track download failed", "Check the error details above", false, err)
	}

	// Validate the downloaded file
	if err := downloader.ValidateAudioFile(trackPath, p.config.FfmpegNameStr); err != nil {
		// Remove corrupted file
		os.Remove(trackPath)
		return err
	}

	return nil
}

// ProcessPaidLstream processes a paid livestream
func (p *Processor) ProcessPaidLstream(query, uguID string, streamParams *models.StreamParams) error {
	q, err := url.ParseQuery(query)
	if err != nil {
		return err
	}

	showId := q["showID"][0]
	if showId == "" {
		return fmt.Errorf("url didn't contain a show id parameter")
	}

	err = p.ProcessVideo(showId, uguID, streamParams, nil, true)
	return err
}

// ProcessCatalogPlist processes a catalog playlist
func (p *Processor) ProcessCatalogPlist(_plistId, legacyToken string, streamParams *models.StreamParams) error {
	plistId, err := resolveCatPlistId(_plistId)
	if err != nil {
		fmt.Println("Failed to resolve playlist ID.")
		return err
	}

	err = p.ProcessPlaylist(plistId, legacyToken, streamParams, true)
	return err
}

// Helper functions
func getAlbumTotal(meta []*models.ArtistMeta) int {
	var total int
	for _, _meta := range meta {
		total += len(_meta.Response.Containers)
	}
	return total
}

func getVideoSku(products []models.Product) int {
	for _, product := range products {
		formatStr := product.FormatStr
		if formatStr == "VIDEO ON DEMAND" || formatStr == "LIVE HD VIDEO" {
			return product.SkuID
		}
	}
	return 0
}

func getLstreamSku(products []*models.ProductFormatList) int {
	for _, product := range products {
		if product.FormatStr == "LIVE HD VIDEO" {
			return product.SkuID
		}
	}
	return 0
}

func getLstreamContainer(containers []*models.AlbArtResp) *models.AlbArtResp {
	for i := len(containers) - 1; i >= 0; i-- {
		c := containers[i]
		if c.AvailabilityTypeStr == "AVAILABLE" && c.ContainerTypeStr == "Show" {
			return c
		}
	}
	return nil
}

func parseLstreamMeta(_meta *models.ArtistMeta) *models.AlbumMeta {
	meta := getLstreamContainer(_meta.Response.Containers)
	parsed := &models.AlbumMeta{
		Response: &models.AlbArtResp{
			ArtistName:        meta.ArtistName,
			ContainerInfo:     meta.ContainerInfo,
			ContainerID:       meta.ContainerID,
			VideoChapters:     meta.VideoChapters,
			Products:          meta.Products,
			ProductFormatList: meta.ProductFormatList,
		},
	}
	return parsed
}

// processTrack processes a track with robust error handling
func (p *Processor) processTrack(trackPath, url string) error {
	return p.downloader.SafeDownloadTrack(trackPath, url, 0) // 0 means unknown size
}

// processTrackWithMetadata processes a track with metadata and robust error handling
func (p *Processor) processTrackWithMetadata(trackPath, url string, metadata *models.TrackMetadata) error {
	// For now, use the existing method but with better error handling
	err := p.downloader.DownloadTrackWithMetadata(trackPath, url, metadata, p.config.FfmpegNameStr)
	if err != nil {
		// Parse FFmpeg errors if they occur
		if strings.Contains(err.Error(), "ffmpeg") {
			errType, msg, guide := downloader.ParseFFmpegError(err, "")
			return models.NewDownloadError(errType, msg, guide, false, err)
		}
		return models.NewDownloadError(models.ErrUnknown, "Track download with metadata failed", "Check FFmpeg installation and file permissions", false, err)
	}
	return nil
}

// processHlsOnly processes HLS-only track with robust error handling
func (p *Processor) processHlsOnly(trackPath, manifestUrl string) error {
	err := p.downloader.HlsOnly(trackPath, manifestUrl, p.config.FfmpegNameStr)
	if err != nil {
		// Parse FFmpeg errors
		if strings.Contains(err.Error(), "ffmpeg") {
			errType, msg, guide := downloader.ParseFFmpegError(err, "")
			return models.NewDownloadError(errType, msg, guide, false, err)
		}
		return models.NewDownloadError(models.ErrFFmpeg, "HLS processing failed", "Check FFmpeg installation and network connection", true, err)
	}
	return nil
}

// processHlsOnlyWithMetadata processes HLS-only track with metadata and robust error handling
func (p *Processor) processHlsOnlyWithMetadata(trackPath, manifestUrl string, metadata *models.TrackMetadata) error {
	err := p.downloader.HlsOnlyWithMetadata(trackPath, manifestUrl, p.config.FfmpegNameStr, metadata)
	if err != nil {
		// Parse FFmpeg errors
		if strings.Contains(err.Error(), "ffmpeg") {
			errType, msg, guide := downloader.ParseFFmpegError(err, "")
			return models.NewDownloadError(errType, msg, guide, false, err)
		}
		return models.NewDownloadError(models.ErrFFmpeg, "HLS processing with metadata failed", "Check FFmpeg installation and network connection", true, err)
	}
	return nil
}

func resolveCatPlistId(plistUrl string) (string, error) {
	// Create a new HTTP client for this request
	httpClient := &http.Client{}
	req, err := httpClient.Get(plistUrl)
	if err != nil {
		return "", models.NewDownloadError(models.ErrNetwork, "Failed to resolve playlist URL", "Check your internet connection", true, err)
	}
	defer req.Body.Close()

	if req.StatusCode != http.StatusOK {
		return "", models.NewDownloadError(models.ErrNetwork, "Playlist URL returned error", "The playlist may not be publicly available", false, errors.New(req.Status))
	}

	location := req.Request.URL.String()
	u, err := url.Parse(location)
	if err != nil {
		return "", models.NewDownloadError(models.ErrUnknown, "Invalid playlist URL format", "Check the playlist URL and try again", false, err)
	}

	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", models.NewDownloadError(models.ErrUnknown, "Failed to parse playlist URL", "The playlist URL format is invalid", false, err)
	}

	resolvedId := q.Get("plGUID")
	if resolvedId == "" {
		return "", models.NewDownloadError(models.ErrUnknown, "Not a catalog playlist", "This appears to be a user playlist, not a catalog playlist", false, nil)
	}

	return resolvedId, nil
}
