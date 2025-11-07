package models

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// Quality represents audio/video quality information
type Quality struct {
	URL      string
	Specs    string
	Format   int
	Extension string
}

// WriteCounter tracks download progress
type WriteCounter struct {
	Total     int64
	TotalStr  string
	Downloaded int64
	Percentage int
	StartTime int64
}

// Write implements io.Writer interface for progress tracking
func (wc *WriteCounter) Write(p []byte) (int, error) {
	var speed int64 = 0
	n := len(p)
	wc.Downloaded += int64(n)

	// Calculate percentage, handling division by zero
	var percentage float64
	if wc.Total > 0 {
		percentage = float64(wc.Downloaded) / float64(wc.Total) * float64(100)
	}
	wc.Percentage = int(percentage)

	toDivideBy := time.Now().UnixMilli() - wc.StartTime
	if toDivideBy != 0 {
		speed = int64(wc.Downloaded) / toDivideBy * 1000
	}
	fmt.Printf("\r%d%% @ %s/s, %s/%s ", wc.Percentage,
		humanize.Bytes(uint64(speed)),
		humanize.Bytes(uint64(wc.Downloaded)), wc.TotalStr)
	return n, nil
}

// AuthResponse represents authentication response
type AuthResponse struct {
	AccessToken string `json:"access_token"`
}

// UserInfo represents user information
type UserInfo struct {
	Sub string `json:"sub"`
}

// SubInfo represents subscription information
type SubInfo struct {
	Plan                Plan     `json:"plan"`
	Promo               Promo    `json:"promo"`
	LegacySubscriptionID string  `json:"legacySubscriptionId"`
	StartedAt           string   `json:"startedAt"`
	EndsAt              string   `json:"endsAt"`
	IsContentAccessible bool     `json:"isContentAccessible"`
	ProductFormatList   []*ProductFormatList `json:"productFormatList"`
}

// Plan represents subscription plan
type Plan struct {
	Description string `json:"description"`
	PlanID      string `json:"planId"`
}

// Promo represents promotional subscription
type Promo struct {
	Plan Plan `json:"plan"`
}

// ProductFormatList represents product format information
type ProductFormatList struct {
	FormatStr string `json:"formatStr"`
	SkuID     int    `json:"skuId"`
}

// StreamParams represents streaming parameters
type StreamParams struct {
	SubscriptionID          string
	SubCostplanIDAccessList string
	UserID                  string
	StartStamp              string
	EndStamp                string
}

// StreamMeta represents stream metadata
type StreamMeta struct {
	StreamLink string `json:"streamLink"`
}

// PurchasedManResp represents purchased manifest response
type PurchasedManResp struct {
	FileURL string `json:"fileUrl"`
}

// AlbumMeta represents album metadata
type AlbumMeta struct {
	Response *AlbArtResp `json:"response"`
}

// AlbArtResp represents album/artist response
type AlbArtResp struct {
	ArtistName        string         `json:"artistName"`
	ContainerInfo     string         `json:"containerInfo"`
	ContainerID       int            `json:"containerId"`
	ContainerTypeStr  string         `json:"containerTypeStr"`
	AvailabilityTypeStr string       `json:"availabilityTypeStr"`
	Tracks            []Track        `json:"tracks"`
	Songs             []Track        `json:"songs"`
	Products          []Product      `json:"products"`
	ProductFormatList []*ProductFormatList `json:"productFormatList"`
	VideoChapters     []interface{}  `json:"videoChapters"`
}

// Track represents a music track
type Track struct {
	TrackID   int    `json:"trackId"`
	SongTitle string `json:"songTitle"`
}

// TrackMetadata represents metadata for tagging audio files
type TrackMetadata struct {
	Title    string
	Artist   string
	Album    string
	TrackNum int
	Year     string
}

// Product represents a product
type Product struct {
	FormatStr string `json:"formatStr"`
	SkuID     int    `json:"skuId"`
}

// PlistMeta represents playlist metadata
type PlistMeta struct {
	Response *PlistResp `json:"response"`
}

// PlistResp represents playlist response
type PlistResp struct {
	PlayListName string       `json:"playListName"`
	Items        []PlistItem  `json:"items"`
}

// PlistItem represents a playlist item
type PlistItem struct {
	Track Track `json:"track"`
}

// ArtistMeta represents artist metadata
type ArtistMeta struct {
	Response *ArtistResp `json:"response"`
}

// ArtistResp represents artist response
type ArtistResp struct {
	Containers []*AlbArtResp `json:"containers"`
}

// Payload represents JWT payload
type Payload struct {
	LegacyToken string `json:"legacyToken"`
	LegacyUguid string `json:"legacyUguid"`
}

// URL patterns for different content types
var RegexStrings = [11]string{
	`^https://play.nugs.net/release/(\d+)$`,
	`^https://play.nugs.net/#/playlists/playlist/(\d+)$`,
	`^https://play.nugs.net/library/playlist/(\d+)$`,
	`(^https://2nu.gs/[a-zA-Z\d]+$)` ,
	`^https://play.nugs.net/#/videos/artist/\d+/.+/(\d+)$`,
	`^https://play.nugs.net/artist/(\d+)(?:/albums|/latest|)$`,
	`^https://play.nugs.net/livestream/(\d+)/exclusive$`,
	`^https://play.nugs.net/watch/livestreams/exclusive/(\d+)$`,
	`^https://play.nugs.net/#/my-webcasts/\d+-(\d+)-\d+-\d+$`,
	`^https://www.nugs.net/on/demandware.store/Sites-NugsNet-Site/d` +
		`efault/(?:Stash-QueueVideo|NugsVideo-GetStashVideo)\?([a-zA-Z0-9=%&-]+$)` ,
	`^https://play.nugs.net/library/webcast/(\d+)$`,
}

// Quality mappings
var QualityMap = map[string]Quality{
	".alac16/": {Specs: "16-bit / 44.1 kHz ALAC", Extension: ".m4a", Format: 1},
	".flac16/": {Specs: "16-bit / 44.1 kHz FLAC", Extension: ".flac", Format: 2},
	".mqa24/":  {Specs: "24-bit / 48 kHz MQA", Extension: ".flac", Format: 3},
	".flac?":   {Specs: "FLAC", Extension: ".flac", Format: 2},
	".s360/":   {Specs: "360 Reality Audio", Extension: ".mp4", Format: 4},
	".aac150/": {Specs: "150 Kbps AAC", Extension: ".m4a", Format: 5},
	".m4a?":   {Specs: "AAC", Extension: ".m4a", Format: 5},
	".m3u8?":  {Extension: ".m4a", Format: 6},
}

// Resolution mappings
var ResolveRes = map[int]string{
	1: "480",
	2: "720",
	3: "1080",
	4: "1440",
	5: "2160",
}

// Track fallback mappings
var TrackFallback = map[int]int{
	1: 2,
	2: 5,
	3: 2,
	4: 3,
}

// Resolution fallback mappings
var ResFallback = map[string]string{
	"720":  "480",
	"1080": "720",
	"1440": "1080",
}

// CheckUrl checks URL pattern and returns ID and media type
func CheckUrl(url string) (string, int) {
	for i, regexStr := range RegexStrings {
		regex := regexp.MustCompile(regexStr)
		match := regex.FindStringSubmatch(url)
		if match != nil {
			return match[1], i
		}
	}
	return "", 0
}

// GetItemTypeName returns human-readable name for media type
func GetItemTypeName(mediaType int) string {
	switch mediaType {
	case 0:
		return "album"
	case 1, 2:
		return "playlist"
	case 3:
		return "catalog_playlist"
	case 4, 10:
		return "video"
	case 5:
		return "artist"
	case 6, 7, 8:
		return "livestream"
	case 9:
		return "paid_livestream"
	default:
		return "unknown"
	}
}

// ExtractLegToken extracts legacy token from JWT
func ExtractLegToken(tokenStr string) (string, string, error) {
	payload := strings.SplitN(tokenStr, ".", 3)[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", "", err
	}

	var obj Payload
	err = json.Unmarshal(decoded, &obj)
	if err != nil {
		return "", "", err
	}

	return obj.LegacyToken, obj.LegacyUguid, nil
}

// ParseTimestamps parses start and end timestamps
func ParseTimestamps(start, end string) (string, string) {
	startTime, _ := time.Parse("01/02/2006 15:04:05", start)
	endTime, _ := time.Parse("01/02/2006 15:04:05", end)
	parsedStart := strconv.FormatInt(startTime.Unix(), 10)
	parsedEnd := strconv.FormatInt(endTime.Unix(), 10)
	return parsedStart, parsedEnd
}

// ParseStreamParams creates stream parameters from subscription info
func ParseStreamParams(userId string, subInfo *SubInfo, isPromo bool) *StreamParams {
	startStamp, endStamp := ParseTimestamps(subInfo.StartedAt, subInfo.EndsAt)
	streamParams := &StreamParams{
		SubscriptionID:          subInfo.LegacySubscriptionID,
		SubCostplanIDAccessList: subInfo.Plan.PlanID,
		UserID:                  userId,
		StartStamp:              startStamp,
		EndStamp:                endStamp,
	}
	if isPromo {
		streamParams.SubCostplanIDAccessList = subInfo.Promo.Plan.PlanID
	} else {
		streamParams.SubCostplanIDAccessList = subInfo.Plan.PlanID
	}
	return streamParams
}

// GetPlan extracts plan description from subscription info
func GetPlan(subInfo *SubInfo) (string, bool) {
	if !reflect.ValueOf(subInfo.Plan).IsZero() {
		return subInfo.Plan.Description, false
	} else {
		return subInfo.Promo.Plan.Description, true
	}
}
