package services

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"streamgo/internal/models"
)

type ALACWAVStreamer struct {
	track     *models.Track
	totalSize int64
	offset    int64
	port      string
	cacheFile string

	cmd    *exec.Cmd
	stdout io.ReadCloser
	cancel context.CancelFunc

	header []byte
}

// NewALACWAVStreamer creates a new ReadSeekCloser that transcodes an ALAC track to WAV on the fly.
func NewALACWAVStreamer(track *models.Track, port string, cacheFile string) *ALACWAVStreamer {
	durationSec := track.Audio.DurationSec
	if durationSec <= 0 {
		durationSec = 300 // fallback 5 mins
	}

	dataSize := int64(durationSec) * 176400 // 44100 Hz * 2 channels * 2 bytes (16-bit)
	totalSize := 44 + dataSize

	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 2)
	binary.LittleEndian.PutUint32(header[24:28], 44100)
	binary.LittleEndian.PutUint32(header[28:32], 176400)
	binary.LittleEndian.PutUint16(header[32:34], 4)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	return &ALACWAVStreamer{
		track:     track,
		totalSize: totalSize,
		port:      port,
		cacheFile: cacheFile,
		header:    header,
	}
}

func (s *ALACWAVStreamer) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = s.offset + offset
	case io.SeekEnd:
		newOffset = s.totalSize + offset
	default:
		return 0, errors.New("invalid whence")
	}

	if newOffset < 0 {
		return 0, errors.New("negative offset")
	}

	if newOffset != s.offset {
		s.closeFFmpeg()
		s.offset = newOffset
	}

	return s.offset, nil
}

func (s *ALACWAVStreamer) closeFFmpeg() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.stdout != nil {
		s.stdout.Close()
		s.stdout = nil
	}
	if s.cmd != nil {
		s.cmd.Wait()
		s.cmd = nil
	}
}

func (s *ALACWAVStreamer) startFFmpeg() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	audioOffset := s.offset
	if audioOffset < 44 {
		audioOffset = 44
	}

	// Align to 4-byte sample frame boundary (16-bit stereo = 2 bytes * 2 channels)
	alignedAudioOffset := 44 + ((audioOffset-44)/4)*4
	timeOffset := float64(alignedAudioOffset-44) / 176400.0

	// Use local FLAC file directly if it exists and is complete; otherwise stream raw track on-the-fly
	inputSource := fmt.Sprintf("http://127.0.0.1:%s/stream/%s?format=raw", s.port, s.track.ID)
	if s.cacheFile != "" {
		if info, err := os.Stat(s.cacheFile); err == nil && info.Size() > 10240 {
			inputSource = s.cacheFile
		}
	}

	args := []string{
		"-hide_banner", "-loglevel", "error",
	}
	if timeOffset > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.6f", timeOffset))
	}
	args = append(args,
		"-i", inputSource,
		"-vn",
		"-c:a", "pcm_s16le",
		"-ar", "44100",
		"-ac", "2",
		"-f", "s16le",
		"pipe:1",
	)

	s.cmd = exec.CommandContext(ctx, "ffmpeg", args...)

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	s.stdout = stdout

	if err := s.cmd.Start(); err != nil {
		cancel()
		return err
	}

	// With -f s16le, FFmpeg writes pure raw PCM audio samples with NO headers.
	// All bytes from s.stdout are pure PCM audio frames.
	return nil
}

func (s *ALACWAVStreamer) Read(p []byte) (n int, err error) {
	if s.offset >= s.totalSize {
		return 0, io.EOF
	}

	bytesRead := 0

	// 1. Serve synthesized header if offset is within [0, 44)
	if s.offset < 44 {
		copied := copy(p, s.header[s.offset:])
		s.offset += int64(copied)
		bytesRead += copied

		if len(p) == bytesRead {
			return bytesRead, nil
		}
		// If buffer still has space, continue to read from ffmpeg
		p = p[copied:]
	}

	// 2. Serve from ffmpeg
	if s.stdout == nil {
		if err := s.startFFmpeg(); err != nil {
			return bytesRead, err
		}
	}

	// Cap read to totalSize
	remaining := s.totalSize - s.offset
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}

	n2, err2 := s.stdout.Read(p)
	if err2 == io.EOF {
		// ffmpeg finished early (e.g., duration was slightly less than estimated).
		// Pad with zeros to fulfill the promised Content-Length.
		s.closeFFmpeg()
		for i := n2; i < len(p); i++ {
			p[i] = 0
		}
		n2 = len(p)
		err2 = nil
	}

	s.offset += int64(n2)
	bytesRead += n2

	return bytesRead, err2
}

func (s *ALACWAVStreamer) Close() error {
	s.closeFFmpeg()
	return nil
}

// GetTotalSize returns the exact pre-calculated size of the WAV stream.
func (s *ALACWAVStreamer) GetTotalSize() int64 {
	return s.totalSize
}
