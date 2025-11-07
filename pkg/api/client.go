package api

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"

	"github.com/grafov/m3u8"
	"github.com/jungletek/Nugs-Downloader/pkg/logger"
)

const (
	devKey         = "x7f54tgbdyc64y656thy47er4"
	clientId       = "Eg7HuH873H65r5rt325UytR5429"
	userAgent      = "NugsNet/3.26.724 (Android; 7.1.2; Asus; ASUS_Z01QD; Scale/2.0; en)"
	userAgentTwo   = "nugsnetAndroid"
	authUrl        = "https://id.nugs.net/connect/token"
	streamApiBase  = "https://streamapi.nugs.net/"
	subInfoUrl     = "https://subscriptions.nugs.net/api/v1/me/subscriptions"
	userInfoUrl    = "https://id.nugs.net/connect/userinfo"
	playerUrl      = "https://play.nugs.net/"
)

var (
	jar, _ = cookiejar.New(nil)
	client = &http.Client{Jar: jar}
)

// Client represents the API client
type Client struct{}

// NewClient creates a new API client
func NewClient() *Client {
	return &Client{}
}

// Auth authenticates with the Nugs API
func (c *Client) Auth(email, pwd string) (string, error) {
	data := url.Values{}
	data.Set("client_id", clientId)
	data.Set("grant_type", "password")
	data.Set("scope", "openid profile email nugsnet:api nugsnet:legacyapi offline_access")
	data.Set("username", email)
	data.Set("password", pwd)

	req, err := http.NewRequest(http.MethodPost, authUrl, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Add("User-Agent", userAgent)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	do, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer do.Body.Close()

	if do.StatusCode != http.StatusOK {
		return "", errors.New(do.Status)
	}

	var obj AuthResponse
	err = json.NewDecoder(do.Body).Decode(&obj)
	if err != nil {
		return "", err
	}

	return obj.AccessToken, nil
}

// GetUserInfo retrieves user information
func (c *Client) GetUserInfo(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, userInfoUrl, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("User-Agent", userAgent)

	do, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer do.Body.Close()

	if do.StatusCode != http.StatusOK {
		return "", errors.New(do.Status)
	}

	var obj UserInfo
	err = json.NewDecoder(do.Body).Decode(&obj)
	if err != nil {
		return "", err
	}

	return obj.Sub, nil
}

// GetSubInfo retrieves subscription information
func (c *Client) GetSubInfo(token string) (*SubInfo, error) {
	req, err := http.NewRequest(http.MethodGet, subInfoUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("User-Agent", userAgent)

	do, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer do.Body.Close()

	if do.StatusCode != http.StatusOK {
		return nil, errors.New(do.Status)
	}

	var obj SubInfo
	err = json.NewDecoder(do.Body).Decode(&obj)
	if err != nil {
		return nil, err
	}

	return &obj, nil
}

// GetAlbumMeta retrieves album metadata
func (c *Client) GetAlbumMeta(albumId string) (*AlbumMeta, error) {
	req, err := http.NewRequest(http.MethodGet, streamApiBase+"api.aspx", nil)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("method", "catalog.container")
	query.Set("containerID", albumId)
	query.Set("vdisp", "1")
	req.URL.RawQuery = query.Encode()
	req.Header.Add("User-Agent", userAgent)

	do, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer do.Body.Close()

	if do.StatusCode != http.StatusOK {
		return nil, errors.New(do.Status)
	}

	var obj AlbumMeta
	err = json.NewDecoder(do.Body).Decode(&obj)
	if err != nil {
		return nil, err
	}

	return &obj, nil
}

// GetPlistMeta retrieves playlist metadata
func (c *Client) GetPlistMeta(plistId, email, legacyToken string, cat bool) (*PlistMeta, error) {
	var path string
	if cat {
		path = "api.aspx"
	} else {
		path = "secureApi.aspx"
	}

	req, err := http.NewRequest(http.MethodGet, streamApiBase+path, nil)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	if cat {
		query.Set("method", "catalog.playlist")
		query.Set("plGUID", plistId)
	} else {
		query.Set("method", "user.playlist")
		query.Set("playlistID", plistId)
		query.Set("developerKey", devKey)
		query.Set("user", email)
		query.Set("token", legacyToken)
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Add("User-Agent", userAgentTwo)

	do, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer do.Body.Close()

	if do.StatusCode != http.StatusOK {
		return nil, errors.New(do.Status)
	}

	var obj PlistMeta
	err = json.NewDecoder(do.Body).Decode(&obj)
	if err != nil {
		return nil, err
	}

	return &obj, nil
}

// GetArtistMeta retrieves artist metadata
func (c *Client) GetArtistMeta(artistId string) ([]*ArtistMeta, error) {
	var allArtistMeta []*ArtistMeta
	offset := 1

	query := url.Values{}
	query.Set("method", "catalog.containersAll")
	query.Set("limit", "100")
	query.Set("artistList", artistId)
	query.Set("availType", "1")
	query.Set("vdisp", "1")

	for {
		req, err := http.NewRequest(http.MethodGet, streamApiBase+"api.aspx", nil)
		if err != nil {
			return nil, err
		}
		query.Set("startOffset", strconv.Itoa(offset))
		req.URL.RawQuery = query.Encode()
		req.Header.Add("User-Agent", userAgent)

		do, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if do.StatusCode != http.StatusOK {
			do.Body.Close()
			return nil, errors.New(do.Status)
		}

		var obj ArtistMeta
		err = json.NewDecoder(do.Body).Decode(&obj)
		do.Body.Close()
		if err != nil {
			return nil, err
		}

		retLen := len(obj.Response.Containers)
		if retLen == 0 {
			break
		}
		allArtistMeta = append(allArtistMeta, &obj)
		offset += retLen
	}

	return allArtistMeta, nil
}

// GetStreamMeta retrieves stream metadata
func (c *Client) GetStreamMeta(trackId, skuId, format int, streamParams *StreamParams) (string, error) {
	req, err := http.NewRequest(http.MethodGet, streamApiBase+"bigriver/subPlayer.aspx", nil)
	if err != nil {
		return "", err
	}

	query := url.Values{}
	if format == 0 {
		query.Set("skuId", strconv.Itoa(skuId))
		query.Set("containerID", strconv.Itoa(trackId))
		query.Set("chap", "1")
	} else {
		query.Set("platformID", strconv.Itoa(format))
		query.Set("trackID", strconv.Itoa(trackId))
	}
	query.Set("app", "1")
	query.Set("subscriptionID", streamParams.SubscriptionID)
	query.Set("subCostplanIDAccessList", streamParams.SubCostplanIDAccessList)
	query.Set("nn_userID", streamParams.UserID)
	query.Set("startDateStamp", streamParams.StartStamp)
	query.Set("endDateStamp", streamParams.EndStamp)
	req.URL.RawQuery = query.Encode()
	req.Header.Add("User-Agent", userAgentTwo)

	do, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer do.Body.Close()

	if do.StatusCode != http.StatusOK {
		return "", errors.New(do.Status)
	}

	var obj StreamMeta
	err = json.NewDecoder(do.Body).Decode(&obj)
	if err != nil {
		return "", err
	}

	return obj.StreamLink, nil
}

// GetPurchasedManUrl retrieves manifest URL for purchased content
func (c *Client) GetPurchasedManUrl(skuID int, showID, userID, uguID string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, streamApiBase+"bigriver/vidPlayer.aspx", nil)
	if err != nil {
		return "", err
	}

	query := url.Values{}
	query.Set("skuId", strconv.Itoa(skuID))
	query.Set("showId", showID)
	query.Set("uguid", uguID)
	query.Set("nn_userID", userID)
	query.Set("app", "1")
	req.URL.RawQuery = query.Encode()
	req.Header.Add("User-Agent", userAgentTwo)

	do, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer do.Body.Close()

	if do.StatusCode != http.StatusOK {
		return "", errors.New(do.Status)
	}

	var obj PurchasedManResp
	err = json.NewDecoder(do.Body).Decode(&obj)
	if err != nil {
		return "", err
	}

	return obj.FileURL, nil
}

// DownloadFile downloads a file from the given URL
func (c *Client) DownloadFile(url, referer string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if referer != "" {
		req.Header.Add("Referer", referer)
	}
	req.Header.Add("User-Agent", userAgent)
	req.Header.Add("Range", "bytes=0-")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, errors.New(resp.Status)
	}

	return resp, nil
}

// GetM3U8Playlist retrieves and parses an M3U8 playlist
func (c *Client) GetM3U8Playlist(url string) (*m3u8.MasterPlaylist, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	playlist, _, err := m3u8.DecodeFrom(resp.Body, true)
	if err != nil {
		return nil, err
	}

	master, ok := playlist.(*m3u8.MasterPlaylist)
	if !ok {
		return nil, errors.New("not a master playlist")
	}

	return master, nil
}

// GetMediaPlaylist retrieves and parses a media playlist
func (c *Client) GetMediaPlaylist(url string) (*m3u8.MediaPlaylist, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	playlist, _, err := m3u8.DecodeFrom(resp.Body, true)
	if err != nil {
		return nil, err
	}

	media, ok := playlist.(*m3u8.MediaPlaylist)
	if !ok {
		return nil, errors.New("not a media playlist")
	}

	return media, nil
}
