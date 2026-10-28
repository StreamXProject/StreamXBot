package metadata

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var albumSlugRegex = regexp.MustCompile(`[^a-z0-9 ]+`)

// Slugify converts text into a canonical lowercase alphanumeric slug with underscores.
// If non-Latin/empty, falls back to u_<sha1_16>.
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

// ArtistID returns the canonical artist ID: artist_<slug>.
func ArtistID(name string) string {
	s := Slugify(name)
	if s == "" {
		return ""
	}
	return "artist_" + s
}

// AlbumID returns the canonical album ID: album_<slug>_<year> or album_<slug>.
func AlbumID(album string, year *int32) string {
	b := Slugify(album)
	if b == "" {
		return ""
	}
	if year != nil && *year >= 1000 && *year <= 2100 {
		return fmt.Sprintf("album_%s_%d", b, *year)
	}
	return fmt.Sprintf("album_%s", b)
}

// NormalizePunctuation standardizes full-width and non-standard punctuation to ASCII equivalents.
func NormalizePunctuation(s string) string {
	if s == "" {
		return ""
	}

	var sb strings.Builder
	sb.Grow(len(s))

	for _, r := range s {
		switch r {
		case '，': // Full-width comma
			sb.WriteString(", ")
		case '／': // Full-width slash
			sb.WriteString(" / ")
		case '；': // Full-width semicolon
			sb.WriteString("; ")
		case '：': // Full-width colon
			sb.WriteString(": ")
		default:
			sb.WriteRune(r)
		}
	}

	// Normalize spaces
	fields := strings.Fields(sb.String())
	res := strings.Join(fields, " ")

	// Fix potential spaced commas/semicolons (e.g. "word , word" -> "word, word")
	res = strings.ReplaceAll(res, " ,", ",")
	res = strings.ReplaceAll(res, " ;", ";")
	res = strings.ReplaceAll(res, ",,", ",")

	return strings.TrimSpace(res)
}

// CleanDuplicatedPhrase checks if a string consists of identical repeated halves
// separated by common delimiters (e.g., ", ", " / ", " - ", "; ").
func CleanDuplicatedPhrase(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 4 {
		return s, false
	}

	delimiters := []string{", ", " / ", " - ", "; ", " // "}

	// 1. Check with spaced delimiters
	for _, delim := range delimiters {
		parts := strings.Split(trimmed, delim)
		n := len(parts)
		if n >= 2 && n%2 == 0 {
			half := n / 2
			firstHalf := parts[:half]
			secondHalf := parts[half:]

			match := true
			for i := 0; i < half; i++ {
				p1 := strings.TrimSpace(firstHalf[i])
				p2 := strings.TrimSpace(secondHalf[i])
				if p1 == "" || !strings.EqualFold(p1, p2) {
					match = false
					break
				}
			}
			if match {
				cleaned := strings.Join(firstHalf, delim)
				return strings.TrimSpace(cleaned), true
			}
		}
	}

	// 2. Check delimiters without spacing (e.g., "A,A" or "A/A")
	delimsNoSpace := []string{",", "/", ";"}
	for _, delim := range delimsNoSpace {
		parts := strings.Split(trimmed, delim)
		n := len(parts)
		if n >= 2 && n%2 == 0 {
			half := n / 2
			firstHalf := parts[:half]
			secondHalf := parts[half:]

			match := true
			for i := 0; i < half; i++ {
				p1 := strings.TrimSpace(firstHalf[i])
				p2 := strings.TrimSpace(secondHalf[i])
				if p1 == "" || !strings.EqualFold(p1, p2) {
					match = false
					break
				}
			}
			if match {
				// Reconstruct with standard spaced delimiter
				cleaned := strings.Join(firstHalf, delim+" ")
				return strings.TrimSpace(cleaned), true
			}
		}
	}

	return s, false
}

// CleanArtist deduplicates stuttered artist tags (e.g. "Shaan, Kailash Kher, Shaan, Kailash Kher").
// Artists are never legitimately repeated entities.
func CleanArtist(artist string) (string, bool) {
	norm := NormalizePunctuation(artist)
	cleaned, changed := CleanDuplicatedPhrase(norm)
	if changed {
		return cleaned, true
	}
	return strings.TrimSpace(norm), false
}

// CleanTitle deduplicates stuttered track titles (e.g. "Chand Sifarish, Chand Sifarish").
// Uses guardrails to ensure legitimate titles with repetitions (such as "Sei Bhalo, Sei Bhalo"
// or "Tonight, Tonight") are preserved:
// 1. If artist is also duplicated, title deduplication is guaranteed to be a corrupted tag.
// 2. If artist is not duplicated, title is only deduplicated if the repeated phrase is complex
//    (>= 3 words or >= 15 characters, such as "Mangal Bhavan Amangal Haari Sitaram Charit").
func CleanTitle(title string, artist string) (string, bool) {
	norm := NormalizePunctuation(title)
	candidate, changed := CleanDuplicatedPhrase(norm)
	if !changed {
		return strings.TrimSpace(norm), false
	}

	// Check if artist also has duplicated halves
	_, artistChanged := CleanArtist(artist)
	if artistChanged {
		// Both title and artist are duplicated: 100% corrupted tag block
		return candidate, true
	}

	// If artist is NOT duplicated, apply false-positive heuristics:
	// Count words and characters in the candidate phrase
	words := strings.Fields(candidate)
	if len(words) >= 3 || len(candidate) >= 15 {
		// Long multi-word phrases (e.g. "Mangal Bhavan Amangal Haari Sitaram Charit") are not intentional titles
		return candidate, true
	}

	// Short phrases (< 3 words and < 15 chars) without artist duplication are preserved
	// (e.g., "Sei Bhalo, Sei Bhalo", "Tonight, Tonight")
	return strings.TrimSpace(norm), false
}

// CleanMetadata sanitizes both title and artist, returning clean strings.
func CleanMetadata(title, artist string) (cleanTitle string, cleanArtist string) {
	cleanArtist, _ = CleanArtist(artist)
	cleanTitle, _ = CleanTitle(title, artist)
	if cleanArtist == "" {
		cleanArtist = strings.TrimSpace(artist)
	}
	if cleanTitle == "" {
		cleanTitle = strings.TrimSpace(title)
	}
	return cleanTitle, cleanArtist
}
