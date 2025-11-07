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

// ProcessAlbum processes an album
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
			logger.GetLogger().Error("Failed to get album metadata", "error", err, "album_id", albumID)
			return err
		}
		meta = _meta.Response
		tracks = meta.Tracks
	}

	trackTotal := len(tracks)
	skuID := getVideoSku(meta.Products)

	if skuID == 0 && trackTotal < 1 {
		return fmt.Errorf("release has no tracks or videos")
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
		fmt.Println("Failed to make album folder.")
		return err
	}

	for trackNum, track := range tracks {
		trackNum++
		err := p.ProcessTrack(albumPath, trackNum, trackTotal, &track, streamParams)
		if err != nil {
			context := map[string]interface{}{
				"album":     meta.ArtistName + " - " + meta.ContainerInfo,
				"track":     track.SongTitle,
				"track_num": trackNum,
				"total":     trackTotal,
			}
			logger.WrapError(err, context)
			logger.GetLogger().Error("Track download failed", "track", track.SongTitle, "album", meta.ContainerInfo)
		}
	}

	return nil
}

// ProcessArtist processes an artist discography
func (p *Processor) ProcessArtist(artistId string, streamParams *models.StreamParams) error {
	meta, err := p.apiClient.GetArtistMeta(artistId)
	if err != nil {
		logger.GetLogger().Error("Failed to get artist metadata", "error", err, "artist_id", artistId)
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
		logger.GetLogger().Error("Failed to get playlist metadata", "error", err, "playlist_id", plistId)
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
			logger.GetLogger().Error("Failed to get video metadata", "error", err, "video_id", videoID)
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

	if isHlsOnly {
		fmt.Println("HLS-only track. Only AAC is available, tags currently unsupported.")
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
		err = p.downloader.HlsOnly(trackPath, chosenQual.URL, p.config.FfmpegNameStr)
	} else {
		err = p.downloader.DownloadTrack(trackPath, chosenQual.URL)
	}

	if err != nil {
		fmt.Println("Failed to download track.")
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

func resolveCatPlistId(plistUrl string) (string, error) {
	// Create a new HTTP client for this request
	httpClient := &http.Client{}
	req, err := httpClient.Get(plistUrl)
	if err != nil {
		return "", err
	}
	defer req.Body.Close()

	if req.StatusCode != http.StatusOK {
		return "", errors.New(req.Status)
	}

	location := req.Request.URL.String()
	u, err := url.Parse(location)
	if err != nil {
		return "", err
	}

	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", err
	}

	resolvedId := q.Get("plGUID")
	if resolvedId == "" {
		return "", errors.New("not a catalog playlist")
	}

	return resolvedId, nil
}
