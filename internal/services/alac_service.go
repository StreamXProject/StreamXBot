package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"streamgo/internal/config"
	"streamgo/internal/logger"
	"streamgo/internal/models"
	"streamgo/internal/telegram"
)

var logALAC = logger.New("alac")

// ALACService handles on-demand lossless ALAC-to-FLAC transcoding, caching, and LRU eviction.
type ALACService struct {
	cfg       *config.Config
	tgService *telegram.Service
	mediaDir  string
	cacheDir  string
	maxBytes  int64
	maxFiles  int

	locks   sync.Map // trackID -> *sync.Mutex
	pruneMu sync.Mutex
}

// NewALACService creates and initializes a new ALACService.
func NewALACService(cfg *config.Config, tg *telegram.Service, mediaDir string) *ALACService {
	if mediaDir == "" {
		mediaDir = "stream_media"
	}
	cacheDir := filepath.Join(mediaDir, "alac_cache")
	_ = os.MkdirAll(cacheDir, 0755)

	maxBytes := int64(5 * 1024 * 1024 * 1024) // 5 GB
	maxFiles := 200

	if cfg != nil {
		if cfg.AlacCacheMaxBytes > 0 {
			maxBytes = cfg.AlacCacheMaxBytes
		}
		if cfg.AlacCacheMaxFiles > 0 {
			maxFiles = cfg.AlacCacheMaxFiles
		}
	}

	return &ALACService{
		cfg:       cfg,
		tgService: tg,
		mediaDir:  mediaDir,
		cacheDir:  cacheDir,
		maxBytes:  maxBytes,
		maxFiles:  maxFiles,
	}
}

// CacheDir returns the absolute or relative path to the ALAC FLAC cache directory.
func (s *ALACService) CacheDir() string {
	return s.cacheDir
}

// IsALACTrack determines if the given track contains an Apple Lossless Audio Codec stream.
func (s *ALACService) IsALACTrack(track *models.Track) bool {
	if track == nil {
		return false
	}

	typeStr := strings.ToLower(strings.TrimSpace(track.EffectiveType()))
	if strings.Contains(typeStr, "alac") {
		return true
	}

	genreStr := strings.ToLower(strings.TrimSpace(track.Audio.Genre))
	if strings.Contains(genreStr, "alac") {
		return true
	}

	mimeStr := strings.ToLower(strings.TrimSpace(track.Telegram.MimeType))
	if strings.Contains(mimeStr, "alac") {
		return true
	}

	nameStr := strings.ToLower(strings.TrimSpace(track.Telegram.FileName))
	if strings.Contains(nameStr, "alac") {
		return true
	}

	// Check if already transcoded into the FLAC cache
	if s.cacheDir != "" {
		cacheFile := filepath.Join(s.cacheDir, fmt.Sprintf("%s.flac", track.ID))
		if info, err := os.Stat(cacheFile); err == nil && info.Size() > 10240 {
			return true
		}
	}

	// M4A/MP4 container analysis:
	// Lossy AAC never has a bit depth in MediaInfo; ALAC is PCM-based lossless with explicit bit depth (16 or 24-bit).
	// In addition, standard 2-channel stereo AAC bitrates max out at 320 kbps, whereas lossless ALAC is typically 500-1400+ kbps.
	isM4A := typeStr == "m4a" || typeStr == "mp4" || mimeStr == "audio/mp4" || mimeStr == "audio/x-m4a" || strings.HasSuffix(nameStr, ".m4a")
	if isM4A {
		if track.Audio.BitDepth != nil && *track.Audio.BitDepth > 0 {
			return true
		}
		if track.Audio.BitrateKbps != nil && *track.Audio.BitrateKbps > 450 {
			return true
		}
		if track.Audio.DurationSec > 0 && track.Telegram.FileSize > 0 {
			calcKbps := (track.Telegram.FileSize * 8) / (int64(track.Audio.DurationSec) * 1000)
			if calcKbps > 450 {
				return true
			}
		}
	}

	return false
}

// ShouldDecodeALAC decides if a track should be served as a transcoded FLAC stream.
func (s *ALACService) ShouldDecodeALAC(r *http.Request, track *models.Track) bool {
	if track == nil {
		return false
	}

	isAlac := s.IsALACTrack(track)

	// Explicit format override via query parameter
	fmtParam := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if fmtParam == "flac" || fmtParam == "decoded" || fmtParam == "transcode" || fmtParam == "pcm" || fmtParam == "wav" {
		return isAlac || !strings.Contains(strings.ToLower(track.Audio.Type), "flac")
	}
	if fmtParam == "raw" || fmtParam == "alac" || fmtParam == "original" || fmtParam == "source" {
		return false
	}

	if !isAlac {
		return false
	}

	// Decode parameter check
	decodeParam := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("decode")))
	if decodeParam == "" {
		decodeParam = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("transcode")))
	}
	if decodeParam == "" {
		decodeParam = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("alac_decode")))
	}
	if decodeParam == "1" || decodeParam == "true" || decodeParam == "yes" || decodeParam == "flac" {
		return true
	}
	if decodeParam == "0" || decodeParam == "false" || decodeParam == "no" {
		return false
	}

	// Check client header for native app bypass
	clientHdr := strings.ToLower(strings.TrimSpace(r.Header.Get("X-StreamX-Client")))
	if clientHdr == "" {
		clientHdr = strings.ToLower(strings.TrimSpace(r.Header.Get("x-streamx-client")))
	}
	if strings.Contains(clientHdr, "native") || strings.Contains(clientHdr, "android-native") || strings.Contains(clientHdr, "exoplayer") {
		return false
	}

	// Browser detection
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	secDest := strings.ToLower(r.Header.Get("Sec-Fetch-Dest"))
	secMode := strings.ToLower(r.Header.Get("Sec-Fetch-Mode"))
	secUA := strings.ToLower(r.Header.Get("Sec-Ch-Ua"))
	secPlatform := strings.ToLower(r.Header.Get("Sec-Ch-Ua-Platform"))

	isBrowser := secDest != "" || secMode != "" || secUA != "" || secPlatform != "" ||
		strings.Contains(ua, "mozilla") || strings.Contains(ua, "chrome") ||
		strings.Contains(ua, "firefox") || strings.Contains(ua, "safari") ||
		strings.Contains(ua, "edge") || strings.Contains(ua, "edg") ||
		strings.Contains(ua, "opera")

	if !isBrowser {
		return false
	}

	// Pure Safari on Apple OS has native ALAC decoding
	isAppleOS := (strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os x") ||
		strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ipod")) &&
		!strings.Contains(ua, "windows") && !strings.Contains(ua, "android")

	if isAppleOS && (strings.Contains(ua, "safari") || strings.Contains(ua, "applewebkit")) &&
		!strings.Contains(ua, "chrome") && !strings.Contains(ua, "edg") && !strings.Contains(ua, "firefox") {
		return false
	}

	// Chromium, Firefox, Windows, Linux, Android browsers all lack ALAC decoder:
	return true
}

// EnsureDecodedFLAC ensures a bit-perfect FLAC transcode of the ALAC track is ready in local cache.
func (s *ALACService) EnsureDecodedFLAC(ctx context.Context, track *models.Track) (string, error) {
	if track == nil {
		return "", errors.New("track is nil")
	}

	targetFile := filepath.Join(s.cacheDir, fmt.Sprintf("%s.flac", track.ID))

	// 1. Check if valid FLAC already exists on disk
	if info, err := os.Stat(targetFile); err == nil && info.Size() > 10240 {
		s.TouchFile(targetFile)
		return targetFile, nil
	}

	// 2. Acquire per-track lock to prevent parallel duplicate transcoding
	actualLock, _ := s.locks.LoadOrStore(track.ID, &sync.Mutex{})
	mtx := actualLock.(*sync.Mutex)
	mtx.Lock()
	defer mtx.Unlock()

	// 3. Double-check after lock
	if info, err := os.Stat(targetFile); err == nil && info.Size() > 10240 {
		s.TouchFile(targetFile)
		return targetFile, nil
	}

	if s.tgService == nil || !s.tgService.IsReady() {
		return "", errors.New("telegram service not ready")
	}

	nowMs := time.Now().UnixMilli()
	pid := os.Getpid()
	tempSrc := filepath.Join(s.cacheDir, fmt.Sprintf("%s.src_%d_%d.m4a", track.ID, pid, nowMs))
	tempFlac := filepath.Join(s.cacheDir, fmt.Sprintf("%s.tmp_%d_%d.flac", track.ID, pid, nowMs))

	defer func() {
		_ = os.Remove(tempSrc)
		_ = os.Remove(tempFlac)
	}()

	logALAC.Infof("[ALAC-Decode] Fetching source media for track %s (%s - %s)", track.ID, track.Audio.Artist, track.Audio.Title)

	// 4. Download source audio from Telegram
	sf, err := os.Create(tempSrc)
	if err != nil {
		return "", fmt.Errorf("failed to create temp source file: %w", err)
	}

	dlErr := s.tgService.DownloadTrack(ctx, track, sf)
	_ = sf.Close()
	if dlErr != nil {
		return "", fmt.Errorf("failed to download source track from telegram: %w", dlErr)
	}

	// 5. Transcode ALAC -> FLAC with -compression_level 0 for low CPU latency
	logALAC.Infof("[ALAC-Decode] Transcoding %s -> %s", tempSrc, tempFlac)
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", tempSrc,
		"-vn",
		"-c:a", "flac",
		"-compression_level", "0",
		"-f", "flac",
		tempFlac,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg transcode failed: %v (output: %s)", err, strings.TrimSpace(string(out)))
	}

	info, err := os.Stat(tempFlac)
	if err != nil || info.Size() <= 10240 {
		return "", fmt.Errorf("transcoded file invalid or too small: %w", err)
	}

	// 6. Atomically move completed FLAC to target path
	if err := os.Rename(tempFlac, targetFile); err != nil {
		return "", fmt.Errorf("failed to rename temp flac to target: %w", err)
	}

	s.TouchFile(targetFile)
	logALAC.Infof("[ALAC-Decode] Transcode completed successfully: %s (%d bytes)", targetFile, info.Size())

	// 7. Non-blocking asynchronous LRU cache pruning
	go s.PruneCacheIfNeeded()

	return targetFile, nil
}

// TouchFile updates the access and modification timestamp of a cached file for LRU tracking.
func (s *ALACService) TouchFile(filePath string) {
	now := time.Now()
	_ = os.Chtimes(filePath, now, now)
}

type cacheEntry struct {
	path    string
	modTime time.Time
	size    int64
}

// PruneCacheIfNeeded removes the least recently accessed files when cache limits are exceeded.
func (s *ALACService) PruneCacheIfNeeded() {
	s.pruneMu.Lock()
	defer s.pruneMu.Unlock()

	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		return
	}

	var files []cacheEntry
	var totalSize int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Only manage completed .flac files; do not touch active in-flight temporary files
		if !strings.HasSuffix(name, ".flac") || strings.Contains(name, ".tmp_") {
			continue
		}

		fullPath := filepath.Join(s.cacheDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, cacheEntry{
			path:    fullPath,
			modTime: info.ModTime(),
			size:    info.Size(),
		})
		totalSize += info.Size()
	}

	if len(files) <= s.maxFiles && totalSize <= s.maxBytes {
		return
	}

	// Sort ascending by modification time (oldest accessed first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	prunedCount := 0
	var prunedBytes int64

	for _, f := range files {
		if len(files)-prunedCount <= s.maxFiles && totalSize <= s.maxBytes {
			break
		}

		if err := os.Remove(f.path); err == nil {
			prunedCount++
			prunedBytes += f.size
			totalSize -= f.size
		}
	}

	if prunedCount > 0 {
		logALAC.Infof("[ALAC-Cache-LRU] Pruned %d stale track(s), freed %.2f MB", prunedCount, float64(prunedBytes)/(1024*1024))
	}
}
