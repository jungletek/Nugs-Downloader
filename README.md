# Nugs-Downloader
Nugs downloader written in Go.
![](https://i.imgur.com/NOsQjnP.png)
![](https://i.imgur.com/BEudufy.png)
[Windows, Linux, macOS, and Android binaries](https://github.com/Sorrow446/Nugs-Downloader/releases)

[![CI](https://github.com/Sorrow446/Nugs-Downloader/actions/workflows/ci.yml/badge.svg)](https://github.com/Sorrow446/Nugs-Downloader/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/Sorrow446/Nugs-Downloader/branch/main/graph/badge.svg)](https://codecov.io/gh/Sorrow446/Nugs-Downloader)
[![Go Report Card](https://goreportcard.com/badge/github.com/Sorrow446/Nugs-Downloader)](https://goreportcard.com/report/github.com/Sorrow446/Nugs-Downloader)

# Setup
Input credentials into config file.
Configure any other options if needed.
|Option|Info|
| --- | --- |
|email|Email address.
|password|Password.
|format|Track download quality. 1 = 16-bit / 44.1 kHz ALAC, 2 = 16-bit / 44.1 kHz FLAC, 3 = 24-bit / 48 kHz MQA, 4 = 360 Reality Audio / best available, 5 = 150 Kbps AAC.
|videoFormat|Video download format. 1 = 480p, 2 = 720p, 3 = 1080p, 4 = 1440p, 5 = 4K / best available. **FFmpeg needed, see below.**
|outPath|Where to download to. Path will be made if it doesn't already exist.
|token|Token to auth with Apple and Google accounts ([how to get token](https://github.com/Sorrow446/Nugs-Downloader/blob/main/token.md)). Ignore if you're using a regular account.
|useFfmpegEnvVar|true = call FFmpeg from environment variable, false = call from script dir.

**FFmpeg is needed for TS -> MP4 losslessly for videos & HLS-only tracks, see below.**  

# FFmpeg Setup
[Windows (gpl)](https://github.com/BtbN/FFmpeg-Builds/releases)    
Linux: `sudo apt install ffmpeg`    
Termux `pkg install ffmpeg`    
Place in Nugs DL's script/binary directory if using FFmpeg binary.

If you don't have root in Linux, you can have Nugs DL look for the binary in the same dir by setting the `useFfmpegEnvVar` option to false.

## Supported Media
|Type|URL example|
| --- | --- |
|Album|`https://play.nugs.net/release/23329`
|Artist|`https://play.nugs.net/#/artist/461/latest`, `https://play.nugs.net/#/artist/461`
|Catalog playlist|`https://2nu.gs/3PmqXLW`
|Exclusive Livestream|`https://play.nugs.net/watch/livestreams/exclusive/30119`
|Purchased Livestream|`https://www.nugs.net/on/demandware.store/Sites-NugsNet-Site/default/Stash-QueueVideo?skuID=624598&showID=30367&perfDate=10-29-2022&artistName=Billy%20Strings&location=10-29-2022%20Exploreasheville%2ecom%20Arena%20Asheville%2c%20NC&format=liveHdStream` Wrap in double quotes on Windows. 
|User playlist|`https://play.nugs.net/#/playlists/playlist/1215400`, `https://play.nugs.net/library/playlist/1261211`
|Video|`https://play.nugs.net/#/videos/artist/1045/Dead%20and%20Company/container/27323` Wrap in double quotes on Windows.
|Webcast|`https://play.nugs.net/#/my-webcasts/5826189-30369-0-624602`

# Usage
Args take priority over the config file.

Download two albums:   
`nugs_dl_x64.exe https://play.nugs.net/release/23329 https://play.nugs.net/release/23790`

Download a single album and from two text files:   
`nugs_dl_x64.exe https://play.nugs.net/release/23329 G:\1.txt G:\2.txt`

Download a user playlist and video:
`nugs_dl_x64.exe https://play.nugs.net/#/playlists/playlist/1215400 "https://play.nugs.net/#/videos/artist/1045/Dead%20and%20Company/container/27323"`

```
 _____                ____                _           _
|   | |_ _ ___ ___   |    \ ___ _ _ _ ___| |___ ___ _| |___ ___
| | | | | | . |_ -|  |  |  | . | | | |   | | . | .'| . | -_|  _|
|_|___|___|_  |___|  |____/|___|_____|_|_|_|___|__,|___|___|_|
          |___|

Usage: nugs_dl_x64.exe [--format FORMAT] [--videoformat VIDEOFORMAT] [--outpath OUTPATH] URLS [URLS ...]

Positional arguments:
  URLS

Options:
  --format FORMAT, -f FORMAT
                         Track download format.
                         1 = 16-bit / 44.1 kHz ALAC
                         2 = 16-bit / 44.1 kHz FLAC
                         3 = 24-bit / 48 kHz MQA
                         4 = 360 Reality Audio / best available
                         5 = 150 Kbps AAC [default: -1]
  --videoformat VIDEOFORMAT, -F VIDEOFORMAT
                         Video download format.
                         1 = 480p
                         2 = 720p
                         3 = 1080p
                         4 = 1440p
                         5 = 4K / best available [default: -1]
  --outpath OUTPATH, -o OUTPATH
                         Where to download to. Path will be made if it doesn't already exist.
  --force-video          Forces video when it co-exists with audio in release URLs.
  --skip-videos          Skips videos in artist URLs.
  --skip-chapters        Skips chapters for videos.
  --help, -h             display this help and exit
  ```
 
# Testing

This project includes comprehensive unit and integration tests to ensure code quality and reliability.

## Running Tests

```bash
# Run all tests
make test

# Run tests with verbose output
make test-verbose

# Run tests with coverage report
make test-coverage

# Run tests with race detection
make test-race
```

## Test Coverage

The test suite covers all major components:

- **pkg/models**: Data structures and utility functions
- **pkg/api**: HTTP client and API interactions (with mocking)
- **pkg/config**: Configuration parsing and validation
- **pkg/logger**: Structured logging functionality
- **pkg/fsutil**: File system operations
- **pkg/downloader**: Core download logic for audio/video content
- **pkg/processor**: High-level content processing orchestration

## CI/CD

The project uses GitHub Actions for continuous integration:

- Automated testing on multiple Go versions (1.21.x, 1.22.x)
- Race condition detection
- Code coverage reporting via Codecov
- Build verification across platforms

## Development

```bash
# Install dependencies
make deps

# Run linter (requires golangci-lint)
make lint

# Build for current platform
make build

# Build for all platforms
make build-all

# Clean build artifacts
make clean
```

# Disclaimer
- I will not be responsible for how you use Nugs Downloader.
- Nugs brand and name is the registered trademark of its respective owner.
- Nugs Downloader has no partnership, sponsorship or endorsement with Nugs.
