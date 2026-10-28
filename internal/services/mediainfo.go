package services

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"streamgo/internal/models"
)

var (
	artistSplitRegex = regexp.MustCompile(`(?i)\s*(?:,|/|&|\s+and\s+|\s+x\s+|\s+feat\.\s+|\s+feat\s+|\s+ft\.\s+|\s+ft\s+)\s*`)
	albumSlugRegex   = regexp.MustCompile(`(?i)[^a-z0-9 ]+`)
	yearRegex        = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	durationHourRe   = regexp.MustCompile(`(?i)(\d+)\s*h`)
	durationMinRe    = regexp.MustCompile(`(?i)(\d+)\s*min`)
	durationSecRe    = regexp.MustCompile(`(?i)(\d+)\s*s`)
	durationMsRe     = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*ms`)
	floatNumberRe    = regexp.MustCompile(`(\d+(?:\.\d+)?)`)
)

// RunMediaInfo executes the system mediainfo binary on the provided file path.
func RunMediaInfo(filePath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "mediainfo", filePath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("mediainfo error: %w (stderr: %s)", err, stderr.String())
	}

	out := stdout.String()
	if out == "" {
		out = stderr.String()
	}
	return out, nil
}

// ParseMediaInfo parses the raw textual output of mediainfo into AudioMeta.
func ParseMediaInfo(output string, fallbackDurationSec int32, fileSize int64) *models.AudioMeta {
	sections := parseMediaInfoSections(output)
	general := sections["general"]
	if general == nil {
		general = make(map[string]string)
	}
	audio := sections["audio"]
	if audio == nil {
		audio = make(map[string]string)
	}

	title := pickBestTitle(general, audio)
	album := getFirst(general, "album", "original source form/name")
	artist := getFirst(general, "performer", "album/performer", "director", "artist")
	if artist == "" {
		artist = audio["performer"]
	}
	title, artist = CleanMetadata(title, artist)
	composer := general["composer"]
	label := getFirst(general, "label", "publisher")
	genre := general["genre"]
	recordedDate := getFirst(general, "recorded date", "recorded date ")

	bitDepth := parseBitDepth(audio["bit depth"])
	bitrateKbps := parseBitrateKbps(getFirst(audio, "bit rate", "overall bit rate", general["overall bit rate"]))
	samplingRateHz := parseSamplingRateHz(audio["sampling rate"])
	year := parseYear(recordedDate)

	// Format / Type normalization
	rawFormat := strings.ToLower(getFirst(audio, "format", "codec id", "format/info", "general format"))
	if rawFormat == "" {
		rawFormat = strings.ToLower(getFirst(general, "format", "codec id"))
	}
	fileType := normalizeAudioFormat(rawFormat)
	// If container is M4A / MP4 / AAC, but bit depth is present (lossless PCM depth 16/24/32 bits), it is ALAC
	if (fileType == "m4a" || fileType == "" || fileType == "aac") && bitDepth != nil && *bitDepth > 0 {
		fileType = "alac"
	}

	durationSec := parseDurationSeconds(getFirst(general, "duration", audio["duration"]))
	if durationSec == 0 && fallbackDurationSec > 0 {
		durationSec = fallbackDurationSec
	}

	// Adjust duration if partial chunk truncated CBR / uncompressed stream
	if fileSize > 3_000_000 {
		var calcDur int32
		if fileType == "pcm" || fileType == "wav" || (samplingRateHz != nil && bitDepth != nil) {
			var sr int64 = 44100
			if samplingRateHz != nil && *samplingRateHz > 0 {
				sr = int64(*samplingRateHz)
			}
			var bd int64 = 16
			if bitDepth != nil && *bitDepth > 0 {
				bd = int64(*bitDepth)
			}
			channels := int64(2)
			chStr := strings.ToLower(getFirst(audio, "channel(s)", general["channel(s)"]))
			if strings.Contains(chStr, "1 channel") {
				channels = 1
			}
			byteRate := (sr * channels * bd) / 8
			if byteRate > 0 {
				calcDur = int32(math.Round(float64(fileSize-44) / float64(byteRate)))
			}
		}
		if calcDur == 0 && bitrateKbps != nil && *bitrateKbps > 0 {
			calcDur = int32(math.Round(float64(fileSize*8) / float64(*bitrateKbps*1000)))
		}

		if calcDur > 0 && (durationSec == 0 || (calcDur > durationSec && (durationSec <= 20 || float64(calcDur)/math.Max(float64(durationSec), 1) >= 2))) {
			durationSec = calcDur
		}
	}

	// Split artists
	artists := SplitArtists(artist)

	// Album ID
	var albumID string
	if album != "" {
		albumID = GenerateAlbumID(album, year)
	}

	meta := &models.AudioMeta{
		Title:          title,
		Album:          album,
		Artist:         artist,
		Composer:       composer,
		Label:          label,
		Genre:          genre,
		Year:           year,
		DurationSec:    durationSec,
		Type:           fileType,
		BitDepth:       bitDepth,
		BitrateKbps:    bitrateKbps,
		SamplingRateHz: samplingRateHz,
		Artists:        artists,
		AlbumID:        albumID,
	}

	return meta
}

func parseMediaInfoSections(output string) map[string]map[string]string {
	sections := make(map[string]map[string]string)
	var current string

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimRight(rawLine, "\r\n")
		header := strings.ToLower(strings.TrimSpace(line))
		switch header {
		case "general", "audio", "video", "text", "image", "menu":
			current = header
			if _, exists := sections[current]; !exists {
				sections[current] = make(map[string]string)
			}
			continue
		}

		if current == "" || !strings.Contains(line, " : ") {
			continue
		}

		parts := strings.SplitN(line, " : ", 2)
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		if k == "" || v == "" {
			continue
		}

		if _, exists := sections[current][k]; !exists {
			sections[current][k] = v
		}
	}

	return sections
}

func pickBestTitle(general, audio map[string]string) string {
	t1 := strings.TrimSpace(getFirst(general, "title", "track name", "track name/position"))
	t2 := strings.TrimSpace(getFirst(audio, "title", "track name"))

	if t1 != "" && !isJunkTitle(t1) {
		return t1
	}
	if t2 != "" && !isJunkTitle(t2) {
		return t2
	}
	if t1 != "" {
		return t1
	}
	return t2
}

func isJunkTitle(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	if t == "" {
		return true
	}
	junk := []string{
		"core media audio",
		"mpeg audio",
		"iso media file produced by google inc.",
		"lavf",
	}
	for _, j := range junk {
		if strings.Contains(t, j) {
			return true
		}
	}
	return false
}

func normalizeAudioFormat(format string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	switch {
	case strings.Contains(f, "flac"):
		return "flac"
	case strings.Contains(f, "alac") || strings.Contains(f, "apple lossless"):
		return "alac"
	case strings.Contains(f, "mpeg") || strings.Contains(f, "layer 3") || f == "mp3":
		return "mp3"
	case strings.Contains(f, "alac"):
		return "alac"
	case strings.Contains(f, "aac") || strings.Contains(f, "mp4"):
		return "m4a"
	case strings.Contains(f, "ogg") || strings.Contains(f, "vorbis") || strings.Contains(f, "opus"):
		return "ogg"
	case strings.Contains(f, "wave") || strings.Contains(f, "pcm") || f == "wav":
		return "wav"
	default:
		if idx := strings.Index(f, " "); idx != -1 {
			return f[:idx]
		}
		return f
	}
}

func parseBitDepth(val string) *int32 {
	if val == "" {
		return nil
	}
	m := floatNumberRe.FindString(val)
	if m == "" {
		return nil
	}
	n, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return nil
	}
	v := int32(math.Round(n))
	return &v
}

func parseBitrateKbps(val string) *int32 {
	if val == "" {
		return nil
	}
	s := strings.ToLower(compactDigits(val))
	m := floatNumberRe.FindString(s)
	if m == "" {
		return nil
	}
	n, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return nil
	}
	var res int32
	if strings.Contains(s, "mb/s") {
		res = int32(math.Round(n * 1000))
	} else if strings.Contains(s, "kb/s") {
		res = int32(math.Round(n))
	} else if strings.Contains(s, "b/s") {
		res = int32(math.Round(n / 1000))
	} else {
		res = int32(math.Round(n))
	}
	return &res
}

func parseSamplingRateHz(val string) *int32 {
	if val == "" {
		return nil
	}
	s := strings.ToLower(compactDigits(val))
	m := floatNumberRe.FindString(s)
	if m == "" {
		return nil
	}
	n, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return nil
	}
	var res int32
	if strings.Contains(s, "khz") {
		res = int32(math.Round(n * 1000))
	} else {
		res = int32(math.Round(n))
	}
	return &res
}

func parseYear(val string) *int32 {
	if val == "" {
		return nil
	}
	m := yearRegex.FindString(val)
	if m == "" {
		return nil
	}
	n, err := strconv.Atoi(m)
	if err != nil || n < 1000 || n > 2100 {
		return nil
	}
	v := int32(n)
	return &v
}

func parseDurationSeconds(val string) int32 {
	if val == "" {
		return 0
	}
	s := strings.ToLower(val)
	var total int32

	if m := durationHourRe.FindStringSubmatch(s); len(m) > 1 {
		h, _ := strconv.Atoi(m[1])
		total += int32(h * 3600)
	}
	if m := durationMinRe.FindStringSubmatch(s); len(m) > 1 {
		min, _ := strconv.Atoi(m[1])
		total += int32(min * 60)
	}
	if m := durationSecRe.FindStringSubmatch(s); len(m) > 1 {
		sec, _ := strconv.Atoi(m[1])
		total += int32(sec)
	}
	if total > 0 {
		return total
	}
	if m := durationMsRe.FindStringSubmatch(s); len(m) > 1 {
		ms, _ := strconv.ParseFloat(m[1], 64)
		return int32(math.Round(ms / 1000))
	}
	return 0
}

func compactDigits(s string) string {
	// Remove space between digits: e.g. "88 200" -> "88200"
	var sb strings.Builder
	runes := []rune(strings.TrimSpace(s))
	for i := 0; i < len(runes); i++ {
		if runes[i] == ' ' && i > 0 && i < len(runes)-1 && unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]) {
			continue
		}
		sb.WriteRune(runes[i])
	}
	return sb.String()
}

func getFirst(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// SplitArtists splits a multi-artist string into individual artist tokens.
func SplitArtists(value string) []string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return []string{}
	}
	raw = strings.ReplaceAll(raw, "(", " ")
	raw = strings.ReplaceAll(raw, ")", " ")
	raw = strings.ReplaceAll(raw, "[", " ")
	raw = strings.ReplaceAll(raw, "]", " ")

	parts := artistSplitRegex.Split(raw, -1)
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool)

	for _, p := range parts {
		token := strings.Trim(strings.TrimSpace(p), " .")
		if token == "" {
			continue
		}
		key := strings.ToLower(token)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, token)
	}
	return out
}

// Slugify normalizes an album title into a URL and ID friendly slug.
func Slugify(value string) string {
	raw := strings.ToLower(strings.TrimSpace(value))
	if raw == "" {
		return ""
	}
	s := strings.ReplaceAll(raw, "÷", " divide ")
	s = strings.ReplaceAll(s, "&", " and ")
	s = strings.ReplaceAll(s, "+", " plus ")

	// NFKD normalization to ASCII
	t := transform.Chain(norm.NFKD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r) // mn: nonspacing marks
	}))
	ascii, _, _ := transform.String(t, s)

	// Keep only alphanumeric and space
	ascii = albumSlugRegex.ReplaceAllString(ascii, " ")
	tokens := strings.Fields(ascii)
	s = strings.Join(tokens, "_")
	s = strings.Trim(s, "_")

	if s != "" {
		return s
	}

	h := sha1.Sum([]byte(raw))
	return fmt.Sprintf("u_%x", h[:8])
}

// GenerateAlbumID builds the canonical album identifier: album_<slug>_<year> or album_<slug>.
func GenerateAlbumID(album string, year *int32) string {
	b := Slugify(album)
	if b == "" {
		return ""
	}
	if year != nil && *year >= 1000 && *year <= 2100 {
		return fmt.Sprintf("album_%s_%d", b, *year)
	}
	return fmt.Sprintf("album_%s", b)
}

// SHA256PrefixFile hashes up to maxBytes of the file at filePath and returns "sha256:<hex>".
func SHA256PrefixFile(filePath string, maxBytes int64) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	_, err = io.CopyN(h, f, maxBytes)
	if err != nil && err != io.EOF {
		return "", err
	}

	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

// BuildMetadataFingerprint creates the canonical fingerprint string matching Python StreamXBot.
func BuildMetadataFingerprint(title, artist, album string, durationSec int32) string {
	t := normalizeText(title)
	a := normalizeText(artist)
	al := normalizeText(album)

	durKey := ""
	if durationSec > 0 {
		bucket := int32(2)
		durKey = strconv.Itoa(int(int32(math.Round(float64(durationSec)/float64(bucket))) * bucket))
	}

	parts := []string{t, a, al, durKey}
	res := strings.Join(parts, "|")
	return strings.Trim(res, "|")
}

func normalizeText(val string) string {
	s := strings.ToLower(val)
	re := regexp.MustCompile(`[^\p{L}\p{N}]+`)
	s = re.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// NormalizeMimeType ensures audio mime types are standardized matching Python StreamXBot.
func NormalizeMimeType(mime string, fileName string) string {
	if fileName != "" {
		fn := strings.ToLower(strings.TrimSpace(fileName))
		switch {
		case strings.HasSuffix(fn, ".wav") || strings.HasSuffix(fn, ".wave"):
			return "audio/wav"
		case strings.HasSuffix(fn, ".flac"):
			return "audio/flac"
		case strings.HasSuffix(fn, ".mp3"):
			return "audio/mpeg"
		case strings.HasSuffix(fn, ".m4a"):
			return "audio/mp4"
		case strings.HasSuffix(fn, ".ogg") || strings.HasSuffix(fn, ".opus"):
			return "audio/ogg"
		case strings.HasSuffix(fn, ".aac"):
			return "audio/aac"
		}
	}

	if mime == "" {
		return "audio/mpeg"
	}

	raw := strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	switch {
	case raw == "audio/flac" || raw == "audio/x-flac" || strings.HasSuffix(raw, "/x-flac"):
		return "audio/flac"
	case raw == "audio/wav" || raw == "audio/x-wav" || raw == "audio/wave":
		return "audio/wav"
	case raw == "audio/mp3" || raw == "audio/mpeg":
		return "audio/mpeg"
	case raw == "audio/m4a" || raw == "audio/x-m4a" || raw == "audio/mp4":
		return "audio/mp4"
	case raw == "audio/ogg" || raw == "application/ogg":
		return "audio/ogg"
	case raw == "audio/aac":
		return "audio/aac"
	default:
		return raw
	}
}
