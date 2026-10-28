package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"streamgo/internal/config"
	"streamgo/internal/logger"
)

var logR2 = logger.New("r2_storage")

// AlbumCovers holds cached public R2 URLs for preview and master covers, as well as external iTunes/Deezer cover URLs.
type AlbumCovers struct {
	CoverURL         string // lightweight thumbnail for list previews
	BigCoverURL      string // master 1000x1000 artwork
	ExternalCoverURL string // external iTunes/Deezer cover URL
}

// R2StorageService provides optional Cloudflare R2 / S3-compatible blob storage
// for persistent, CDN-delivered artwork thumbnails.
type R2StorageService struct {
	accountID       string
	accessKeyID     string
	secretAccessKey string
	bucketName      string
	publicURL       string
	httpClient      *http.Client

	mu              sync.RWMutex
	albumCoverCache map[string]AlbumCovers // albumID -> AlbumCovers
}

// NewR2StorageService creates a new R2StorageService from application config.
func NewR2StorageService(cfg *config.Config) *R2StorageService {
	if cfg == nil || !cfg.HasR2() {
		return &R2StorageService{
			albumCoverCache: make(map[string]AlbumCovers),
		}
	}

	publicURL := strings.TrimRight(cfg.R2PublicURL, "/")
	if publicURL == "" {
		// Fallback to standard R2 dev or custom endpoint format
		publicURL = fmt.Sprintf("https://%s.r2.cloudflarestorage.com/%s", cfg.R2AccountID, cfg.R2BucketName)
	}

	return &R2StorageService{
		accountID:       cfg.R2AccountID,
		accessKeyID:     cfg.R2AccessKeyID,
		secretAccessKey: cfg.R2SecretAccessKey,
		bucketName:      cfg.R2BucketName,
		publicURL:       publicURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		albumCoverCache: make(map[string]AlbumCovers),
	}
}

// IsConfigured returns true if all required R2 credentials are present.
func (s *R2StorageService) IsConfigured() bool {
	return s != nil && s.accountID != "" && s.accessKeyID != "" && s.secretAccessKey != "" && s.bucketName != ""
}

// GetAlbumCover returns the cached public R2 master/big cover URL for an album, if present.
func (s *R2StorageService) GetAlbumCover(albumID string) string {
	if s == nil || albumID == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry := s.albumCoverCache[albumID]
	if entry.BigCoverURL != "" {
		return entry.BigCoverURL
	}
	return entry.CoverURL
}

// GetAlbumCovers returns the cached preview thumbnail and master artwork R2 URLs for an album.
func (s *R2StorageService) GetAlbumCovers(albumID string) (coverURL, bigCoverURL string) {
	if s == nil || albumID == "" {
		return "", ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry := s.albumCoverCache[albumID]
	return entry.CoverURL, entry.BigCoverURL
}

// GetAlbumExternalCover returns the cached external (iTunes/Deezer) cover URL for an album, if present.
func (s *R2StorageService) GetAlbumExternalCover(albumID string) string {
	if s == nil || albumID == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.albumCoverCache[albumID].ExternalCoverURL
}

// SetAlbumExternalCover caches the external (iTunes/Deezer) cover URL for an album.
func (s *R2StorageService) SetAlbumExternalCover(albumID, externalCoverURL string) {
	if s == nil || albumID == "" || externalCoverURL == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.albumCoverCache[albumID]
	entry.ExternalCoverURL = externalCoverURL
	s.albumCoverCache[albumID] = entry
}

// SetAlbumCover caches the public R2 cover URL for an album.
func (s *R2StorageService) SetAlbumCover(albumID, coverURL string) {
	if s == nil || albumID == "" || coverURL == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.albumCoverCache[albumID]
	entry.BigCoverURL = coverURL
	if entry.CoverURL == "" {
		entry.CoverURL = coverURL
	}
	s.albumCoverCache[albumID] = entry
}

// SetAlbumCovers caches both the preview and master R2 cover URLs for an album.
func (s *R2StorageService) SetAlbumCovers(albumID, coverURL, bigCoverURL string) {
	if s == nil || albumID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.albumCoverCache[albumID]
	if coverURL != "" {
		entry.CoverURL = coverURL
	}
	if bigCoverURL != "" {
		entry.BigCoverURL = bigCoverURL
	}
	s.albumCoverCache[albumID] = entry
}

// MaxCoverPayloadBytes defines the maximum allowed file size for cover art uploads (5 MB).
const MaxCoverPayloadBytes = 5 * 1024 * 1024

// DetectImageFormat inspects magic header bytes to determine image MIME type and file extension.
// Returns ok=false if data does not match known image formats (JPEG, PNG, WebP).
func DetectImageFormat(data []byte) (mimeType string, ext string, ok bool) {
	if len(data) < 4 {
		return "", "", false
	}
	// JPEG: 0xFF 0xD8 0xFF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg", "jpg", true
	}
	// PNG: 0x89 'P' 'N' 'G'
	if data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return "image/png", "png", true
	}
	// WebP: RIFF....WEBP
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		if _, _, _, err := SanitizeAndValidateWebP(data); err == nil {
			return "image/webp", "webp", true
		}
		return "", "", false
	}
	return "", "", false
}

// UploadCover uploads artwork bytes to Cloudflare R2 using standard S3 SigV4 authentication.
// Rejects non-image payloads (audio files, corrupt streams) and oversized payloads > 5 MB.
// Returns the public CDN URL for the uploaded object.
func (s *R2StorageService) UploadCover(ctx context.Context, hash string, data []byte, contentType string) (string, error) {
	if !s.IsConfigured() {
		return "", fmt.Errorf("r2 storage is not configured")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("cannot upload empty data")
	}
	if len(data) > MaxCoverPayloadBytes {
		return "", fmt.Errorf("refusing to upload oversized cover (%d bytes, max %d bytes)", len(data), MaxCoverPayloadBytes)
	}

	detectedMime, ext, isImg := DetectImageFormat(data)
	if !isImg {
		preview := data
		if len(preview) > 8 {
			preview = preview[:8]
		}
		return "", fmt.Errorf("refusing to upload non-image or invalid payload to R2 (detected header: %x)", preview)
	}

	if detectedMime == "image/webp" {
		sanitized, _, _, err := SanitizeAndValidateWebP(data)
		if err != nil {
			return "", fmt.Errorf("refusing to upload invalid/corrupted WebP to R2: %w", err)
		}
		data = sanitized
	}

	if contentType == "" {
		contentType = detectedMime
	}
	key := fmt.Sprintf("covers/%s.%s", hash, ext)

	endpointHost := fmt.Sprintf("%s.r2.cloudflarestorage.com", s.accountID)
	reqURL := fmt.Sprintf("https://%s/%s/%s", endpointHost, s.bucketName, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	cacheControl := "public, max-age=31536000, immutable"

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Cache-Control", cacheControl)
	req.Header.Set("Host", endpointHost)

	// Sign request with AWS SigV4
	now := time.Now().UTC()
	signS3Request(req, data, s.accessKeyID, s.secretAccessKey, "auto", "s3", now)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("r2 upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return "", fmt.Errorf("r2 upload returned status %d", resp.StatusCode)
	}

	publicURL := fmt.Sprintf("%s/%s", s.publicURL, key)
	logR2.Infof("Successfully uploaded artwork to R2: %s (%.1f KB)", key, float64(len(data))/1024.0)
	return publicURL, nil
}

// DeleteCover removes an artwork object from Cloudflare R2 bucket.
func (s *R2StorageService) DeleteCover(ctx context.Context, key string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("r2 storage is not configured")
	}
	key = strings.TrimPrefix(key, "/")
	endpointHost := fmt.Sprintf("%s.r2.cloudflarestorage.com", s.accountID)
	reqURL := fmt.Sprintf("https://%s/%s/%s", endpointHost, s.bucketName, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	req.Header.Set("Host", endpointHost)

	now := time.Now().UTC()
	signS3Request(req, nil, s.accessKeyID, s.secretAccessKey, "auto", "s3", now)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("r2 delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("r2 delete returned status %d", resp.StatusCode)
	}

	logR2.Infof("Successfully deleted artwork from R2: %s", key)
	return nil
}

// DeleteCoverByURL parses the R2 key from a public CDN URL and removes the object.
// Returns nil if R2 is not configured or if URL is empty.
func (s *R2StorageService) DeleteCoverByURL(ctx context.Context, rawURL string) error {
	if s == nil || !s.IsConfigured() || strings.TrimSpace(rawURL) == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid cover url: %w", err)
	}
	key := strings.TrimPrefix(parsed.Path, "/")
	if key == "" {
		return nil
	}
	return s.DeleteCover(ctx, key)
}

// signS3Request signs an HTTP request using AWS Signature Version 4 (SigV4).
func signS3Request(req *http.Request, payload []byte, accessKey, secretKey, region, service string, t time.Time) {
	dateStamp := t.Format("20060102")
	amzDate := t.Format("20060102T150405Z")

	payloadHashBytes := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	// Build Canonical Headers (must be lowercase and sorted alphabetically)
	host := req.Header.Get("Host")
	cacheControl := req.Header.Get("Cache-Control")
	contentType := req.Header.Get("Content-Type")

	canonicalHeaders := fmt.Sprintf(
		"cache-control:%s\ncontent-type:%s\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		cacheControl, contentType, host, payloadHash, amzDate,
	)
	signedHeaders := "cache-control;content-type;host;x-amz-content-sha256;x-amz-date"

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		"", // query string (empty for simple PUT)
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	canonicalRequestHash := sha256Hex([]byte(canonicalRequest))

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		canonicalRequestHash,
	}, "\n")

	signingKey := getSignatureKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", authHeader)
}

func hmacSHA256(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func getSignatureKey(key, dateStamp, regionName, serviceName string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+key), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(regionName))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}
