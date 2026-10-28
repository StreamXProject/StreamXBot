package services

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"streamgo/internal/logger"
)

var logArtwork = logger.New("artwork")

// ArtworkService extracts and compresses album artwork from audio file bytes.
type ArtworkService struct{}

// NewArtworkService creates a new ArtworkService.
func NewArtworkService() *ArtworkService {
	return &ArtworkService{}
}

// ExtractArtwork attempts to extract embedded album artwork from an audio chunk.
// It detects container formats (MP4/ALAC/M4A, FLAC, ID3/MP3) automatically or via hint.
func (s *ArtworkService) ExtractArtwork(data []byte, formatHint string) ([]byte, string, error) {
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty audio data")
	}

	hint := strings.ToLower(strings.TrimSpace(formatHint))

	// 1. FLAC Container
	if hint == "flac" || (len(data) >= 4 && string(data[:4]) == "fLaC") {
		img, mime, _, _, err := ExtractFLACPicture(data)
		if err == nil && len(img) > 0 {
			return img, mime, nil
		}
	}

	// 2. MP4 / ALAC / M4A Container (inspect for covr or moov)
	if hint == "alac" || hint == "m4a" || bytes.Contains(data, []byte("covr")) || bytes.Contains(data, []byte("moov")) {
		img, mime, err := ExtractMP4Cover(data)
		if err == nil && len(img) > 0 {
			return img, mime, nil
		}
	}

	// 3. ID3v2 (MP3, AAC)
	if hint == "mp3" || (len(data) >= 3 && string(data[:3]) == "ID3") || bytes.Contains(data, []byte("APIC")) {
		img, mime, err := ExtractID3Picture(data)
		if err == nil && len(img) > 0 {
			return img, mime, nil
		}
	}

	return nil, "", fmt.Errorf("no embedded artwork found in audio chunk")
}

// IsArtworkTruncated inspects container metadata headers (FLAC, MP4/ALAC, or ID3)
// to detect if embedded artwork or container tags extend beyond the currently downloaded slice.
func (s *ArtworkService) IsArtworkTruncated(data []byte, formatHint string) bool {
	if len(data) < 4 {
		return false
	}
	hint := strings.ToLower(strings.TrimSpace(formatHint))

	// 1. FLAC Container
	if hint == "flac" || (len(data) >= 4 && string(data[:4]) == "fLaC") {
		if len(data) >= 4 && string(data[:4]) == "fLaC" {
			offset := 4
			for offset+4 <= len(data) {
				header := data[offset]
				isLast := (header & 0x80) != 0
				blockType := header & 0x7F
				length := int(binary.BigEndian.Uint32([]byte{0, data[offset+1], data[offset+2], data[offset+3]}))
				offset += 4

				if offset+length > len(data) {
					// Either a picture block is cut off, or subsequent metadata blocks (potentially picture) are cut off
					return true
				}
				if blockType == 6 {
					// PICTURE block is fully present in this slice (or corrupted)
					return false
				}
				if isLast {
					// Reached last metadata block without encountering PICTURE block
					return false
				}
				offset += length
			}
			return true // Slice ended inside block header before isLast
		}
	}

	// 2. MP4 / ALAC / M4A Container
	if hint == "alac" || hint == "m4a" || bytes.Contains(data, []byte("covr")) || bytes.Contains(data, []byte("moov")) || bytes.Contains(data, []byte("ftyp")) {
		if needed, ok := InspectMP4CoverNeeds(data); ok {
			return needed > int64(len(data))
		}
		if moovIdx := bytes.Index(data, []byte("moov")); moovIdx >= 4 {
			moovSize := int64(binary.BigEndian.Uint32(data[moovIdx-4 : moovIdx]))
			if moovSize == 1 && moovIdx+12 <= len(data) {
				moovSize = int64(binary.BigEndian.Uint64(data[moovIdx+4 : moovIdx+12]))
			}
			if moovSize > 0 && int64(moovIdx-4)+moovSize > int64(len(data)) {
				return true
			}
		}
	}

	// 3. ID3v2 (MP3, AAC)
	if hint == "mp3" || (len(data) >= 3 && string(data[:3]) == "ID3") || bytes.Contains(data, []byte("APIC")) {
		if len(data) >= 10 && string(data[:3]) == "ID3" {
			tagSize := int(data[6]&0x7F)<<21 | int(data[7]&0x7F)<<14 | int(data[8]&0x7F)<<7 | int(data[9]&0x7F)
			if int64(10+tagSize) > int64(len(data)) {
				return true
			}
		}
		if apicIdx := bytes.Index(data, []byte("APIC")); apicIdx != -1 {
			if apicIdx+10 > len(data) {
				return true
			}
			isV4 := len(data) >= 4 && data[0] == 'I' && data[1] == 'D' && data[2] == '3' && data[3] == 4
			var frameSize int
			if isV4 {
				b := data[apicIdx+4 : apicIdx+8]
				frameSize = int(b[0]&0x7F)<<21 | int(b[1]&0x7F)<<14 | int(b[2]&0x7F)<<7 | int(b[3]&0x7F)
			} else {
				frameSize = int(binary.BigEndian.Uint32(data[apicIdx+4 : apicIdx+8]))
			}
			if apicIdx+10+frameSize > len(data) {
				return true
			}
		}
	}

	return false
}

// SanitizeAndValidateWebP validates WebP container structure and payload integrity.
// If the WebP bitstream has missing RIFF length or trailing muxer bytes (common when muxers cannot seek),
// it repairs the header and strips trailing bytes.
// Returns the sanitized byte slice, image dimensions, or an error if invalid/corrupt.
func SanitizeAndValidateWebP(data []byte) ([]byte, int, int, error) {
	if len(data) < 20 {
		return nil, 0, 0, fmt.Errorf("data too short for WebP (%d bytes)", len(data))
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, 0, 0, fmt.Errorf("missing RIFF/WEBP magic bytes")
	}

	fourCC := string(data[12:16])
	chunkLen := binary.LittleEndian.Uint32(data[16:20])

	switch fourCC {
	case "VP8 ":
		// Simple lossy WebP
		expectedLen := 12 + 8 + int(chunkLen) + int(chunkLen%2)
		if len(data) < expectedLen {
			return nil, 0, 0, fmt.Errorf("truncated VP8 chunk: have %d bytes, need %d", len(data), expectedLen)
		}

		result := data
		if len(result) > expectedLen {
			result = result[:expectedLen]
		}

		riffSize := binary.LittleEndian.Uint32(result[4:8])
		expectedRiffSize := uint32(len(result) - 8)
		if riffSize != expectedRiffSize {
			newBuf := make([]byte, len(result))
			copy(newBuf, result)
			binary.LittleEndian.PutUint32(newBuf[4:8], expectedRiffSize)
			result = newBuf
		}

		payload := result[20:]
		if len(payload) < 10 {
			return nil, 0, 0, fmt.Errorf("VP8 payload too short (%d bytes)", len(payload))
		}
		// Bit 0 of payload[0] is frame type (0 = keyframe, 1 = interframe)
		if (payload[0] & 0x01) != 0 {
			return nil, 0, 0, fmt.Errorf("VP8 is not a keyframe")
		}
		// Start code bytes 3, 4, 5: 0x9d 0x01 0x2a
		if payload[3] != 0x9d || payload[4] != 0x01 || payload[5] != 0x2a {
			return nil, 0, 0, fmt.Errorf("invalid VP8 keyframe start code")
		}
		width := int(binary.LittleEndian.Uint16(payload[6:8]) & 0x3fff)
		height := int(binary.LittleEndian.Uint16(payload[8:10]) & 0x3fff)
		if width <= 0 || height <= 0 {
			return nil, 0, 0, fmt.Errorf("invalid VP8 dimensions: %dx%d", width, height)
		}
		return result, width, height, nil

	case "VP8L":
		// Lossless WebP
		expectedLen := 12 + 8 + int(chunkLen) + int(chunkLen%2)
		if len(data) < expectedLen {
			return nil, 0, 0, fmt.Errorf("truncated VP8L chunk: have %d bytes, need %d", len(data), expectedLen)
		}

		result := data
		if len(result) > expectedLen {
			result = result[:expectedLen]
		}

		riffSize := binary.LittleEndian.Uint32(result[4:8])
		expectedRiffSize := uint32(len(result) - 8)
		if riffSize != expectedRiffSize {
			newBuf := make([]byte, len(result))
			copy(newBuf, result)
			binary.LittleEndian.PutUint32(newBuf[4:8], expectedRiffSize)
			result = newBuf
		}

		payload := result[20:]
		if len(payload) < 5 || payload[0] != 0x2f {
			return nil, 0, 0, fmt.Errorf("invalid VP8L signature byte")
		}
		b0, b1, b2, b3 := uint32(payload[1]), uint32(payload[2]), uint32(payload[3]), uint32(payload[4])
		val := b0 | (b1 << 8) | (b2 << 16) | (b3 << 24)
		width := int(val&0x3fff) + 1
		height := int((val>>14)&0x3fff) + 1
		if width <= 0 || height <= 0 {
			return nil, 0, 0, fmt.Errorf("invalid VP8L dimensions: %dx%d", width, height)
		}
		return result, width, height, nil

	case "VP8X":
		// Extended WebP
		if len(data) < 30 {
			return nil, 0, 0, fmt.Errorf("VP8X header too short")
		}
		result := data
		riffSize := binary.LittleEndian.Uint32(result[4:8])
		expectedRiffSize := uint32(len(result) - 8)
		if riffSize != expectedRiffSize {
			newBuf := make([]byte, len(result))
			copy(newBuf, result)
			binary.LittleEndian.PutUint32(newBuf[4:8], expectedRiffSize)
			result = newBuf
		}
		width := int(uint32(result[24]) | uint32(result[25])<<8 | uint32(result[26])<<16) + 1
		height := int(uint32(result[27]) | uint32(result[28])<<8 | uint32(result[29])<<16) + 1
		if width <= 0 || height <= 0 {
			return nil, 0, 0, fmt.Errorf("invalid VP8X dimensions: %dx%d", width, height)
		}
		return result, width, height, nil

	default:
		return nil, 0, 0, fmt.Errorf("unsupported WebP chunk type: %q", fourCC)
	}
}

// CompressToWebP compresses and resizes raw image bytes into WebP format using FFmpeg.
// It writes to a temporary file so FFmpeg can seek and finalize standard RIFF headers,
// then sanitizes and verifies the output bitstream to ensure zero corruption.
// maxDim sets maximum width/height (aspect ratio preserved). quality is 0-100 (default 80).
func (s *ArtworkService) CompressToWebP(ctx context.Context, raw []byte, maxDim int, quality int) ([]byte, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty image bytes")
	}
	if maxDim <= 0 {
		maxDim = 1000
	}
	if quality <= 0 || quality > 100 {
		quality = 80
	}

	start := time.Now()

	tmpOut, err := os.CreateTemp("", "streamgo_webp_*.webp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp webp file: %w", err)
	}
	tmpPath := tmpOut.Name()
	_ = tmpOut.Close()
	defer os.Remove(tmpPath)

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", "pipe:0",
		"-vf", fmt.Sprintf("scale='min(%d,iw)':-1", maxDim),
		"-c:v", "libwebp",
		"-quality", fmt.Sprintf("%d", quality),
		tmpPath,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdin = bytes.NewReader(raw)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg webp compression failed: %w: %s", err, errBuf.String())
	}

	outBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read webp output file: %w", err)
	}

	sanitized, w, h, err := SanitizeAndValidateWebP(outBytes)
	if err != nil {
		return nil, fmt.Errorf("generated webp failed validation: %w", err)
	}

	dur := time.Since(start)
	logArtwork.Debugf("Compressed artwork to WebP (%dx%d): %d bytes -> %d bytes in %v",
		w, h, len(raw), len(sanitized), dur.Round(time.Millisecond))

	return sanitized, nil
}

// ExtractMP4Cover extracts embedded cover art from MP4 / M4A / ALAC atom structure (ilst -> covr -> data).
func ExtractMP4Cover(data []byte) ([]byte, string, error) {
	covrIdx := bytes.Index(data, []byte("covr"))
	if covrIdx == -1 {
		return nil, "", fmt.Errorf("covr atom not found in slice")
	}

	dataIdx := bytes.Index(data[covrIdx:], []byte("data"))
	if dataIdx == -1 {
		return nil, "", fmt.Errorf("data atom not found inside covr")
	}
	absDataIdx := covrIdx + dataIdx

	if absDataIdx < 4 || absDataIdx+16 > len(data) {
		return nil, "", fmt.Errorf("truncated data atom header")
	}

	dataLen := int(binary.BigEndian.Uint32(data[absDataIdx-4 : absDataIdx]))
	typeIndicator := binary.BigEndian.Uint32(data[absDataIdx+4 : absDataIdx+8])

	imgStart := absDataIdx + 12
	payloadLen := dataLen - 16

	if payloadLen > 0 && imgStart+payloadLen <= len(data) {
		imgData := data[imgStart : imgStart+payloadLen]
		mime := "image/jpeg"
		if typeIndicator == 14 || bytes.HasPrefix(imgData, []byte("\x89PNG")) {
			mime = "image/png"
		}
		return imgData, mime, nil
	}

	// Fallback scanning for JPEG magic markers if atom length was slightly off
	if imgStart < len(data) {
		jpegStart := bytes.Index(data[imgStart:], []byte("\xff\xd8\xff"))
		if jpegStart != -1 {
			absJpegStart := imgStart + jpegStart
			jpegEnd := bytes.Index(data[absJpegStart:], []byte("\xff\xd9"))
			if jpegEnd != -1 {
				return data[absJpegStart : absJpegStart+jpegEnd+2], "image/jpeg", nil
			}
		}
	}

	return nil, "", fmt.Errorf("could not extract valid image payload from covr atom (payloadLen=%d, available=%d)", payloadLen, len(data)-imgStart)
}

// InspectMP4CoverNeeds determines the total byte length required from file start to capture the entire cover atom.
func InspectMP4CoverNeeds(data []byte) (int64, bool) {
	covrIdx := bytes.Index(data, []byte("covr"))
	if covrIdx == -1 {
		return 0, false
	}

	dataIdx := bytes.Index(data[covrIdx:], []byte("data"))
	if dataIdx == -1 {
		return 0, false
	}
	absDataIdx := covrIdx + dataIdx

	if absDataIdx < 4 || absDataIdx+16 > len(data) {
		return 0, false
	}

	dataLen := int64(binary.BigEndian.Uint32(data[absDataIdx-4 : absDataIdx]))
	imgStart := int64(absDataIdx + 12)
	payloadLen := dataLen - 16

	if payloadLen > 0 {
		return imgStart + payloadLen, true
	}
	return 0, false
}

// ExtractFLACPicture extracts the METADATA_BLOCK_PICTURE from raw FLAC chunk bytes.
func ExtractFLACPicture(data []byte) ([]byte, string, int, int, error) {
	if len(data) < 4 || string(data[:4]) != "fLaC" {
		return nil, "", 0, 0, fmt.Errorf("invalid FLAC header")
	}

	offset := 4
	for offset < len(data) {
		if offset+4 > len(data) {
			break
		}

		header := data[offset]
		isLast := (header & 0x80) != 0
		blockType := header & 0x7F
		length := int(binary.BigEndian.Uint32([]byte{0, data[offset+1], data[offset+2], data[offset+3]}))
		offset += 4

		if offset+length > len(data) {
			return nil, "", 0, 0, fmt.Errorf("truncated FLAC metadata block")
		}

		if blockType == 6 { // METADATA_BLOCK_PICTURE
			picData := data[offset : offset+length]
			if len(picData) < 32 {
				return nil, "", 0, 0, fmt.Errorf("picture block too short")
			}

			mimeLen := int(binary.BigEndian.Uint32(picData[4:8]))
			if 8+mimeLen+4 > len(picData) {
				return nil, "", 0, 0, fmt.Errorf("corrupt picture mime")
			}
			mime := string(picData[8 : 8+mimeLen])

			descOffset := 8 + mimeLen
			descLen := int(binary.BigEndian.Uint32(picData[descOffset : descOffset+4]))

			metaOffset := descOffset + 4 + descLen
			if metaOffset+16 > len(picData) {
				return nil, "", 0, 0, fmt.Errorf("corrupt picture dimensions")
			}
			width := int(binary.BigEndian.Uint32(picData[metaOffset : metaOffset+4]))
			height := int(binary.BigEndian.Uint32(picData[metaOffset+4 : metaOffset+8]))

			dataLenOffset := metaOffset + 16
			if dataLenOffset+4 > len(picData) {
				return nil, "", 0, 0, fmt.Errorf("corrupt picture data length")
			}
			dataLen := int(binary.BigEndian.Uint32(picData[dataLenOffset : dataLenOffset+4]))

			imgStart := dataLenOffset + 4
			if imgStart+dataLen > len(picData) {
				return nil, "", 0, 0, fmt.Errorf("corrupt picture payload")
			}

			return picData[imgStart : imgStart+dataLen], mime, width, height, nil
		}

		if isLast {
			break
		}
		offset += length
	}

	return nil, "", 0, 0, fmt.Errorf("no picture block found in FLAC metadata")
}

// ExtractID3Picture extracts the APIC (attached picture) frame from ID3v2 tag bytes.
// Supports both ID3v2.3 (32-bit big-endian frame size) and ID3v2.4 (28-bit syncsafe frame size),
// respects declared image MIME types (e.g. image/png vs image/jpeg), and cleanly truncates at image EOF.
func ExtractID3Picture(data []byte) ([]byte, string, error) {
	apicIdx := bytes.Index(data, []byte("APIC"))
	if apicIdx == -1 {
		return nil, "", fmt.Errorf("APIC frame not found")
	}

	// APIC frame header is 10 bytes:
	// 'APIC' (4) + Size (4) + Flags (2)
	frameHeader := data[apicIdx:]
	if len(frameHeader) < 10 {
		return nil, "", fmt.Errorf("truncated APIC header")
	}

	// Check ID3 major version (ID3v2.4 uses syncsafe frame sizes)
	isV4 := len(data) >= 4 && data[0] == 'I' && data[1] == 'D' && data[2] == '3' && data[3] == 4
	var frameSize int
	if isV4 {
		b := frameHeader[4:8]
		frameSize = int(b[0]&0x7F)<<21 | int(b[1]&0x7F)<<14 | int(b[2]&0x7F)<<7 | int(b[3]&0x7F)
	} else {
		frameSize = int(binary.BigEndian.Uint32(frameHeader[4:8]))
	}

	payload := frameHeader[10:]
	if frameSize > 0 && 10+frameSize <= len(frameHeader) {
		payload = frameHeader[10 : 10+frameSize]
	}
	if len(payload) < 4 {
		return nil, "", fmt.Errorf("APIC payload too short")
	}

	// Payload layout:
	// [1 byte text encoding]
	// [MIME type string (null-terminated)]
	// [1 byte picture type]
	// [Description string (null-terminated)]
	// [Raw image bytes...]
	mimeEnd := bytes.IndexByte(payload[1:], 0)
	if mimeEnd == -1 {
		return nil, "", fmt.Errorf("corrupt APIC mime terminator")
	}
	mime := strings.ToLower(string(payload[1 : 1+mimeEnd]))

	var imgStart int = -1
	var detectedMime string

	// Respect declared MIME type first to avoid false-positive matches
	// (e.g. \xff\xd8\xff occurring inside PNG deflate streams)
	if strings.Contains(mime, "png") {
		if pIdx := bytes.Index(payload, []byte("\x89PNG\r\n\x1a\n")); pIdx != -1 {
			imgStart = pIdx
			detectedMime = "image/png"
		}
	} else if strings.Contains(mime, "jpeg") || strings.Contains(mime, "jpg") {
		if jIdx := bytes.Index(payload, []byte("\xff\xd8\xff")); jIdx != -1 {
			imgStart = jIdx
			detectedMime = "image/jpeg"
		}
	}

	// Fallback to whichever valid image header appears first
	if imgStart == -1 {
		pIdx := bytes.Index(payload, []byte("\x89PNG\r\n\x1a\n"))
		jIdx := bytes.Index(payload, []byte("\xff\xd8\xff"))
		if pIdx != -1 && (jIdx == -1 || pIdx < jIdx) {
			imgStart = pIdx
			detectedMime = "image/png"
		} else if jIdx != -1 {
			imgStart = jIdx
			detectedMime = "image/jpeg"
		}
	}

	if imgStart == -1 {
		return nil, "", fmt.Errorf("could not locate image magic bytes in APIC frame")
	}

	rawImg := payload[imgStart:]
	// Truncate at image EOF marker if present to avoid trailing garbage
	if detectedMime == "image/png" {
		if endIdx := bytes.Index(rawImg, []byte("IEND")); endIdx != -1 && endIdx+8 <= len(rawImg) {
			rawImg = rawImg[:endIdx+8]
		}
	} else if detectedMime == "image/jpeg" {
		if endIdx := bytes.Index(rawImg, []byte("\xff\xd9")); endIdx != -1 {
			rawImg = rawImg[:endIdx+2]
		}
	}

	return rawImg, detectedMime, nil
}
