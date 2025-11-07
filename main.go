package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"main/pkg/api"
	"main/pkg/config"
	"main/pkg/downloader"
	"main/pkg/fsutil"
	"main/pkg/logger"
	"main/pkg/models"
	"main/pkg/processor"
)

func main() {
	fmt.Println(`
 _____                ____                _           _
|   | |_ _ ___ ___   |    \ ___ _ _ _ ___| |___ ___ _| |___ ___
| | | | | | . |_ -|  |  |  | . | | | |   | | . | .'| . | -_|  _|
|_|___|___|_  |___|  |____/|___|_____|_|_|_|___|__,|___|___|_|
	  |___|
`)

	// Change to script directory
	scriptDir, err := getScriptDir()
	if err != nil {
		panic(err)
	}
	err = os.Chdir(scriptDir)
	if err != nil {
		panic(err)
	}

	// Parse configuration
	cfg, err := config.ParseCfg()
	if err != nil {
		logger.GetLogger().Error("Failed to parse config/args", "error", err)
		os.Exit(1)
	}

	// Create output directory
	err = fsutil.MakeDirs(cfg.OutPath)
	if err != nil {
		logger.GetLogger().Error("Failed to make output folder", "error", err)
		os.Exit(1)
	}

	// Initialize API client
	apiClient := api.NewClient()

	// Authenticate if no token provided
	var token string
	if cfg.Token == "" {
		token, err = apiClient.Auth(cfg.Email, cfg.Password)
		if err != nil {
			logger.GetLogger().Error("Failed to authenticate", "error", err)
			os.Exit(1)
		}
	} else {
		token = cfg.Token
	}

	// Get user info
	userId, err := apiClient.GetUserInfo(token)
	if err != nil {
		logger.GetLogger().Error("Failed to get user info", "error", err)
		os.Exit(1)
	}

	// Get subscription info
	subInfo, err := apiClient.GetSubInfo(token)
	if err != nil {
		logger.GetLogger().Error("Failed to get subscription info", "error", err)
		os.Exit(1)
	}

	// Extract legacy token
	legacyToken, uguID, err := models.ExtractLegToken(token)
	if err != nil {
		logger.GetLogger().Error("Failed to extract legacy token", "error", err)
		os.Exit(1)
	}

	// Get plan description
	planDesc, isPromo := models.GetPlan(subInfo)
	if !subInfo.IsContentAccessible {
		planDesc = "no active subscription"
	}
	fmt.Println("Signed in successfully - " + planDesc + "\n")

	// Parse stream parameters
	streamParams := models.ParseStreamParams(userId, subInfo, isPromo)

	// Initialize downloader and processor
	downloader := downloader.NewDownloader(apiClient, cfg)
	processor := processor.NewProcessor(apiClient, downloader, cfg)

	// Process URLs
	albumTotal := len(cfg.Urls)
	for albumNum, url := range cfg.Urls {
		fmt.Printf("Item %d of %d:\n", albumNum+1, albumTotal)

		itemId, mediaType := models.CheckUrl(url)
		if itemId == "" {
			fmt.Println("Invalid URL:", url)
			continue
		}

		var itemErr error
		switch mediaType {
		case 0:
			itemErr = processor.ProcessAlbum(itemId, streamParams, nil)
		case 1, 2:
			itemErr = processor.ProcessPlaylist(itemId, legacyToken, streamParams, false)
		case 3:
			itemErr = processor.ProcessCatalogPlist(itemId, legacyToken, streamParams)
		case 4, 10:
			itemErr = processor.ProcessVideo(itemId, "", streamParams, nil, false)
		case 5:
			itemErr = processor.ProcessArtist(itemId, streamParams)
		case 6, 7, 8:
			itemErr = processor.ProcessVideo(itemId, "", streamParams, nil, true)
		case 9:
			itemErr = processor.ProcessPaidLstream(itemId, uguID, streamParams)
		}

		if itemErr != nil {
			context := map[string]interface{}{
				"item_type": models.GetItemTypeName(mediaType),
				"item_id":   itemId,
				"item_num":  albumNum + 1,
				"total":     albumTotal,
				"url":       url,
			}
			logger.WrapError(itemErr, context)
			logger.GetLogger().Error("Item processing failed",
				"type", models.GetItemTypeName(mediaType),
				"id", itemId,
				"url", url)
		}
	}
}

// getScriptDir returns the directory of the script
func getScriptDir() (string, error) {
	var (
		ok    bool
		err   error
		fname string
	)

	runFromSrc := wasRunFromSrc()
	if runFromSrc {
		_, fname, _, ok = runtime.Caller(0)
		if !ok {
			return "", fmt.Errorf("failed to get script filename")
		}
	} else {
		fname, err = os.Executable()
		if err != nil {
			return "", err
		}
	}

	return filepath.Dir(fname), nil
}

// wasRunFromSrc checks if the program was run from source
func wasRunFromSrc() bool {
	buildPath := filepath.Join(os.TempDir(), "go-build")
	return strings.HasPrefix(os.Args[0], buildPath)
}
