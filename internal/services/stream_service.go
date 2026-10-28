package services

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gotd/td/tg"

	"streamgo/internal/logger"
	"streamgo/internal/models"
	"streamgo/internal/repository"
	"streamgo/internal/telegram"
)

var logStream = logger.New("stream")

var rangeRegex = regexp.MustCompile(`(?i)^bytes\s*=\s*(\d*)\s*-\s*(\d*)$`)

// ErrRangeNotSatisfiable is returned when the requested byte range is outside the file bounds.
var ErrRangeNotSatisfiable = errors.New("range not satisfiable")

// StreamService coordinates audio range parsing and streaming.
type StreamService struct {
	repo        repository.TrackRepository
	tgService   *telegram.Service
	alacService *ALACService
	mediaDir    string
}

// NewStreamService creates a new StreamService.
func NewStreamService(repo repository.TrackRepository, tg *telegram.Service) *StreamService {
	mediaDir := "stream_media"
	_ = os.MkdirAll(mediaDir, 0755)
	_ = os.MkdirAll(filepath.Join(mediaDir, "alac_cache"), 0755)

	return &StreamService{
		repo:      repo,
		tgService: tg,
		mediaDir:  mediaDir,
	}
}

// SetALACService attaches the ALACService to StreamService.
func (s *StreamService) SetALACService(alac *ALACService) {
	s.alacService = alac
}

// ALACService returns the attached ALACService.
func (s *StreamService) ALACService() *ALACService {
	return s.alacService
}

// ByteRange defines start and end byte offsets.
type ByteRange struct {
	Start  int64
	End    int64
	Length int64
}

// ParseRange parses an HTTP Range header string into byte boundaries.
func ParseRange(rangeHeader string, totalSize int64) (*ByteRange, error) {
	rangeHeader = strings.TrimSpace(rangeHeader)
	if rangeHeader == "" {
		return nil, nil
	}

	matches := rangeRegex.FindStringSubmatch(rangeHeader)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid range format: %s", rangeHeader)
	}

	startStr := strings.TrimSpace(matches[1])
	endStr := strings.TrimSpace(matches[2])

	if startStr == "" && endStr == "" {
		return nil, fmt.Errorf("empty range boundaries")
	}

	var start, end int64

	if startStr == "" {
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid range offset: %s", endStr)
		}
		start = totalSize - n
		if start < 0 {
			start = 0
		}
		end = totalSize - 1
	} else {
		var err error
		start, err = strconv.ParseInt(startStr, 10, 64)
		if err != nil || start < 0 {
			return nil, fmt.Errorf("invalid range start: %s", startStr)
		}

		if endStr == "" {
			end = totalSize - 1
		} else {
			end, err = strconv.ParseInt(endStr, 10, 64)
			if err != nil || end < start {
				return nil, fmt.Errorf("invalid range end: %s", endStr)
			}
			if end >= totalSize {
				end = totalSize - 1
			}
		}
	}

	if totalSize > 0 && start >= totalSize {
		return nil, ErrRangeNotSatisfiable
	}

	return &ByteRange{
		Start:  start,
		End:    end,
		Length: end - start + 1,
	}, nil
}

// StreamTrack handles streaming audio for a track ID with HTTP Range support.
func (s *StreamService) StreamTrack(w http.ResponseWriter, r *http.Request, trackID string) {
	ctx := r.Context()

	track, err := s.repo.GetByID(ctx, trackID)
	if err != nil {
		http.Error(w, "Failed to retrieve track", http.StatusInternalServerError)
		return
	}
	if track == nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	// 1. Determine total size
	totalSize := track.Audio.FileSize
	if totalSize <= 0 {
		totalSize = track.Telegram.FileSize
	}
	if totalSize <= 0 {
		totalSize = 10 * 1024 * 1024
	}

	// 2. Determine mime type and normalize vendor mime types
	audioType := strings.ToLower(strings.TrimSpace(track.Audio.Type))
	mimeType := track.Audio.MimeType
	if mimeType == "" {
		mimeType = track.Telegram.MimeType
	}
	switch {
	case audioType == "flac" || mimeType == "audio/x-flac" || mimeType == "audio/flac":
		mimeType = "audio/flac"
	case audioType == "wav" || mimeType == "audio/x-wav" || mimeType == "audio/wav":
		mimeType = "audio/wav"
	case audioType == "mp3" || audioType == "mpeg" || mimeType == "audio/mp3":
		mimeType = "audio/mpeg"
	case audioType == "alac" || audioType == "m4a" || audioType == "aac" || mimeType == "audio/x-m4a" || mimeType == "audio/mp4":
		mimeType = "audio/mp4"
	case audioType == "ogg" || audioType == "opus":
		mimeType = "audio/ogg"
	}
	if mimeType == "" {
		mimeType = "audio/mpeg"
	}

	// 3. Clean filename for Content-Disposition with proper audio extension
	cleanBase := ""
	if track.Audio.Title != "" {
		if track.Audio.Artist != "" {
			cleanBase = fmt.Sprintf("%s - %s", track.Audio.Artist, track.Audio.Title)
		} else {
			cleanBase = track.Audio.Title
		}
	} else if track.Telegram.FileName != "" {
		cleanBase = strings.TrimSuffix(track.Telegram.FileName, filepath.Ext(track.Telegram.FileName))
	} else {
		cleanBase = track.ID
	}

	ext := audioType
	if ext == "alac" {
		ext = "m4a"
	}
	if ext == "" {
		switch mimeType {
		case "audio/flac":
			ext = "flac"
		case "audio/wav":
			ext = "wav"
		case "audio/mp4":
			ext = "m4a"
		case "audio/ogg":
			ext = "ogg"
		default:
			ext = "mp3"
		}
	}
	filename := fmt.Sprintf("%s.%s", cleanBase, ext)

	serveCachedFLAC := func(filePath string) bool {
		info, statErr := os.Stat(filePath)
		if statErr != nil || info.Size() <= 10240 {
			return false
		}
		f, openErr := os.Open(filePath)
		if openErr != nil {
			return false
		}
		defer f.Close()

		if s.alacService != nil {
			s.alacService.TouchFile(filePath)
		}

		cleanBase := track.Audio.Title
		if track.Audio.Artist != "" {
			cleanBase = fmt.Sprintf("%s - %s", track.Audio.Artist, track.Audio.Title)
		}
		if cleanBase == "" {
			cleanBase = track.Telegram.FileName
		}
		if cleanBase == "" {
			cleanBase = track.ID
		}
		cleanBase = strings.TrimSuffix(cleanBase, filepath.Ext(cleanBase))
		flacFilename := fmt.Sprintf("%s.flac", cleanBase)

		fallback := strings.ReplaceAll(flacFilename, `"`, `_`)
		encoded := url.QueryEscape(flacFilename)
		w.Header().Set("Content-Type", "audio/flac")
		w.Header().Set("Accept-Ranges", "bytes")
		if w.Header().Get("Content-Disposition") == "" {
			w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"; filename*=UTF-8''%s`, fallback, encoded))
		}

		go func(tid string) {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.repo.IncrementPlayCount(timeoutCtx, tid)
		}(track.ID)

		http.ServeContent(w, r, flacFilename, info.ModTime(), f)
		return true
	}

	// 3. If track is ALAC (or FLAC requested) and needs decoding, serve consistent WAV stream
	if s.alacService != nil && s.alacService.ShouldDecodeALAC(r, track) {
		cacheFile := filepath.Join(s.mediaDir, "alac_cache", fmt.Sprintf("%s.flac", track.ID))

		// Kick off background FLAC cache if not already running
		go s.alacService.EnsureDecodedFLAC(context.Background(), track)

		// Serve on-the-fly WAV stream with full seek support (reads from local cacheFile if ready, else on-the-fly)
		wavStreamer := NewALACWAVStreamer(track, s.alacService.cfg.Port, cacheFile)
		defer wavStreamer.Close()

		// Increment play count asynchronously
		go func(tid string) {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.repo.IncrementPlayCount(timeoutCtx, tid)
		}(track.ID)

		cleanBase := track.Audio.Title
		if track.Audio.Artist != "" {
			cleanBase = fmt.Sprintf("%s - %s", track.Audio.Artist, track.Audio.Title)
		}
		if cleanBase == "" {
			cleanBase = track.Telegram.FileName
		}
		if cleanBase == "" {
			cleanBase = track.ID
		}
		cleanBase = strings.TrimSuffix(cleanBase, filepath.Ext(cleanBase))
		wavFilename := fmt.Sprintf("%s.wav", cleanBase)

		fallback := strings.ReplaceAll(wavFilename, `"`, `_`)
		encoded := url.QueryEscape(wavFilename)

		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"; filename*=UTF-8''%s`, fallback, encoded))

		// http.ServeContent handles all Range parsing and chunking automatically!
		http.ServeContent(w, r, wavFilename, time.Now(), wavStreamer)
		return
	}

	// For non-decoded tracks, do not hijack with cached FLAC if raw format was explicitly requested
	rawParam := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if rawParam != "raw" && rawParam != "alac" && rawParam != "original" && rawParam != "source" {
		cacheFile := filepath.Join(s.mediaDir, "alac_cache", fmt.Sprintf("%s.flac", track.ID))
		if serveCachedFLAC(cacheFile) {
			return
		}
	}

	// 4. Handle Range Header
	rangeHeader := r.Header.Get("Range")
	var byteRange *ByteRange
	if rangeHeader != "" {
		var err error
		byteRange, err = ParseRange(rangeHeader, totalSize)
		if errors.Is(err, ErrRangeNotSatisfiable) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
			http.Error(w, "Requested Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		// If err != nil (malformed syntax), RFC 7233 says ignore Range and serve full content (byteRange == nil)
	}

	// 5. Check Telegram client connectivity BEFORE writing any response headers.
	// If headers (200/206 + Content-Length) are committed and then no bytes follow,
	// the browser's <audio> element fires MEDIA_ERR_SRC_NOT_SUPPORTED.
	if s.tgService == nil {
		http.Error(w, "Telegram streaming service is not configured", http.StatusServiceUnavailable)
		return
	}
	if !s.tgService.IsReady() {
		// Wait briefly for a connecting client (e.g. startup race)
		deadline := time.After(8 * time.Second)
		ticker := time.NewTicker(250 * time.Millisecond)
		waitDone := false
		for !waitDone {
			select {
			case <-deadline:
				waitDone = true
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				if s.tgService.IsReady() {
					waitDone = true
				}
			}
		}
		ticker.Stop()
		if !s.tgService.IsReady() {
			logStream.Warnf("Telegram client not ready for track %s; returning 503", trackID)
			http.Error(w, "Telegram streaming service is not ready", http.StatusServiceUnavailable)
			return
		}
	}

	// 6. Setup Response Headers
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", mimeType)
	if w.Header().Get("Content-Disposition") == "" {
		fallback := strings.ReplaceAll(filename, `"`, `_`)
		encoded := url.QueryEscape(filename)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"; filename*=UTF-8''%s`, fallback, encoded))
	}

	status := http.StatusOK
	var contentLength int64 = totalSize

	if byteRange != nil {
		status = http.StatusPartialContent
		contentLength = byteRange.Length
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", byteRange.Start, byteRange.End, totalSize))
	}
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))

	// If HEAD request, emit headers and finish immediately
	if r.Method == http.MethodHead {
		w.WriteHeader(status)
		return
	}

	// 7. Asynchronously increment play count
	go func(tid string) {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.repo.IncrementPlayCount(timeoutCtx, tid)
	}(track.ID)

	w.WriteHeader(status)

	// Stream chunks directly from Telegram to HTTP Response
	s.streamFromTelegram(ctx, w, track, byteRange, totalSize)
}

func (s *StreamService) streamFromTelegram(
	ctx context.Context,
	w http.ResponseWriter,
	track *models.Track,
	byteRange *ByteRange,
	totalSize int64,
) {
	var startOffset int64 = 0
	var bytesRemaining int64 = totalSize

	if byteRange != nil {
		startOffset = byteRange.Start
		bytesRemaining = byteRange.Length
	}

	logStream.Infof("streaming track %s: offset=%d, length=%d", track.ID, startOffset, bytesRemaining)

	// 1. Prefer acquiring a worker that already has a mapped file_id
	var worker *telegram.ClientWorker
	workers := s.tgService.Workers()
	if track.Telegram.FileIDs != nil {
		for _, w := range workers {
			if w.Ready {
				wID := strconv.FormatInt(w.ID, 10)
				if wID == "0" || wID == "" {
					if parts := strings.Split(w.Token, ":"); len(parts) > 0 {
						wID = parts[0]
					}
				}
				if fid, ok := track.Telegram.FileIDs[wID]; ok && fid != "" {
					worker = s.tgService.AcquireWorkerForBot(wID)
					break
				}
			}
		}
	}
	if worker == nil {
		worker = s.tgService.AcquireWorker()
	}
	defer s.tgService.ReleaseWorker(worker)

	workerIDStr := strconv.FormatInt(worker.ID, 10)
	if workerIDStr == "0" || workerIDStr == "" {
		if parts := strings.Split(worker.Token, ":"); len(parts) > 0 {
			workerIDStr = parts[0]
		}
	}

	fileID := ""
	if track.Telegram.FileIDs != nil && workerIDStr != "0" {
		fileID = track.Telegram.FileIDs[workerIDStr]
	}

	fetchFresh := func() string {
		if worker.Ready && track.CacheChatID != 0 && track.CacheMessageID != 0 {
			if fid, err := worker.FetchFileIDForMessage(ctx, track.CacheChatID, track.CacheMessageID); err == nil && fid != "" {
				go func(tid, wid, f string) {
					_ = s.repo.UpdateWorkerFileID(context.Background(), tid, wid, f)
				}(track.ID, workerIDStr, fid)
				return fid
			}
		}
		if worker.Ready && track.SourceChatID != 0 && track.SourceMessageID != 0 {
			if fid, err := worker.FetchFileIDForMessage(ctx, track.SourceChatID, track.SourceMessageID); err == nil && fid != "" {
				go func(tid, wid, f string) {
					_ = s.repo.UpdateWorkerFileID(context.Background(), tid, wid, f)
				}(track.ID, workerIDStr, fid)
				return fid
			}
		}
		return ""
	}

	// Fallback: If this worker does not have a cached file_id, fetch on the fly
	if fileID == "" {
		fileID = fetchFresh()
		if fileID == "" && track.Telegram.FileID != "" {
			primary := s.tgService.PrimaryWorker()
			if primary != nil && primary.Ready && primary != worker {
				s.tgService.ReleaseWorker(worker)
				worker = primary
				workerIDStr = strconv.FormatInt(worker.ID, 10)
				atomic.AddInt64(&worker.Workload, 1)
			}
			fileID = track.Telegram.FileID
		}
	}

	if fileID == "" {
		logStream.Errorf("track %s has no telegram file_id", track.ID)
		return
	}

	decoded, err := telegram.DecodeFileID(fileID)
	if err != nil {
		logStream.Errorf("failed to decode file_id for track %s: %v", track.ID, err)
		return
	}

	location := &tg.InputDocumentFileLocation{
		ID:            decoded.MediaID,
		AccessHash:    decoded.AccessHash,
		FileReference: decoded.FileReference,
	}

	flusher, _ := w.(http.Flusher)

	// MTProto UploadGetFile requires offset to be divisible by 4096 (4KB),
	// and no request may cross a 1MB (1048576 byte) boundary.
	const maxChunkSize = 512 * 1024 // 512 KB
	const oneMB int64 = 1024 * 1024
	chunkOffset := (startOffset / 4096) * 4096
	skipBytes := startOffset - chunkOffset
	refreshed := false

	for bytesRemaining > 0 {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Calculate maximum allowed limit up to the next 1MB boundary
		bytesToBoundary := oneMB - (chunkOffset % oneMB)
		currentLimit := maxChunkSize
		if int64(currentLimit) > bytesToBoundary {
			currentLimit = int(bytesToBoundary)
		}

		req := &tg.UploadGetFileRequest{
			Precise:  true,
			Location: location,
			Offset:   chunkOffset,
			Limit:    currentLimit,
		}

		var res tg.UploadFileClass
		var fetchErr error
		for attempt := 0; attempt < 3; attempt++ {
			res, fetchErr = worker.API.UploadGetFile(ctx, req)
			if fetchErr == nil {
				break
			}
			if !refreshed && strings.Contains(strings.ToUpper(fetchErr.Error()), "FILE_REFERENCE") {
				logStream.Warnf("File reference expired for track %s at offset %d; refreshing...", track.ID, chunkOffset)
				if fresh := fetchFresh(); fresh != "" {
					if freshDecoded, freshErr := telegram.DecodeFileID(fresh); freshErr == nil {
						location = &tg.InputDocumentFileLocation{
							ID:            freshDecoded.MediaID,
							AccessHash:    freshDecoded.AccessHash,
							FileReference: freshDecoded.FileReference,
						}
						req.Location = location
						refreshed = true
						continue
					}
				}
			}
			if errors.Is(fetchErr, context.Canceled) || ctx.Err() != nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}

		if fetchErr != nil {
			logStream.Warnf("UploadGetFile failed after retries for track %s at offset %d: %v", track.ID, chunkOffset, fetchErr)
			return
		}

		var data []byte
		switch f := res.(type) {
		case *tg.UploadFile:
			data = f.Bytes
		default:
			logStream.Warnf("unexpected UploadGetFile result type %T for track %s", res, track.ID)
			return
		}

		rawLen := len(data)
		if rawLen == 0 {
			// Telegram EOF reached
			break
		}

		// Handle sub-4KB unaligned start offset on first chunk
		if skipBytes > 0 {
			if int64(rawLen) <= skipBytes {
				skipBytes -= int64(rawLen)
				chunkOffset += int64(rawLen)
				continue
			}
			data = data[skipBytes:]
			skipBytes = 0
		}

		// Cap data to bytesRemaining
		if int64(len(data)) > bytesRemaining {
			data = data[:bytesRemaining]
		}

		// Sanitize corrupted FLAC metadata blocks (e.g. picture block with mime_len <= 0) on the first chunk
		if startOffset == 0 && chunkOffset == 0 && len(data) >= 8 && string(data[:4]) == "fLaC" {
			sanitizeFLACHeader(data)
		}

		written, writeErr := w.Write(data)
		if flusher != nil {
			flusher.Flush()
		}
		if writeErr != nil {
			// Client disconnected (e.g. stopped playback, navigated away, or seeked)
			return
		}

		bytesRemaining -= int64(written)
		chunkOffset += int64(rawLen)

		// If Telegram returned fewer bytes than requested limit, it was the final chunk of the file
		if rawLen < currentLimit {
			break
		}
	}
}

// GetTrack retrieves a track by ID.
func (s *StreamService) GetTrack(ctx context.Context, id string) (*models.Track, error) {
	return s.repo.GetByID(ctx, id)
}

// WarmTrack pre-resolves track information, checks Telegram worker availability, and pre-transcodes ALAC tracks.
func (s *StreamService) WarmTrack(ctx context.Context, id string) {
	track, err := s.repo.GetByID(ctx, id)
	if err != nil || track == nil {
		return
	}
	logStream.Infof("Prewarming track %s (%s - %s)", track.ID, track.Audio.Artist, track.Audio.Title)
	if s.alacService != nil && s.alacService.IsALACTrack(track) {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			_, _ = s.alacService.EnsureDecodedFLAC(bgCtx, track)
		}()
	}
}

// sanitizeFLACHeader inspects FLAC metadata blocks and converts corrupt picture blocks (mime_len <= 0)
// into harmless PADDING blocks so that Chromium/Edge demuxers don't abort with AVERROR_INVALIDDATA.
func sanitizeFLACHeader(data []byte) {
	if len(data) < 8 || string(data[:4]) != "fLaC" {
		return
	}
	pos := 4
	for pos+4 <= len(data) {
		b0 := data[pos]
		isLast := (b0 & 0x80) != 0
		blockType := b0 & 0x7F
		length := int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
		if blockType == 6 { // PICTURE block
			if pos+4+8 <= len(data) {
				mimeLen := int(binary.BigEndian.Uint32(data[pos+4+4 : pos+4+8]))
				if mimeLen <= 0 {
					data[pos] = (b0 & 0x80) | 1 // Convert to block type 1 (PADDING)
					logStream.Infof("Sanitized corrupted FLAC picture block (mime_len=%d) to PADDING at offset %d", mimeLen, pos)
				}
			}
		}
		pos += 4 + length
		if isLast {
			break
		}
	}
}

// IsTelegramReady returns whether the Telegram streaming service has at least one connected worker.
func (s *StreamService) IsTelegramReady() bool {
	return s.tgService != nil && s.tgService.IsReady()
}
