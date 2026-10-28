package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// resolveDistDir locates the WebX frontend build directory.
func resolveDistDir() string {
	candidates := []string{
		"dist",
		"../WebX/dist",
		"WebX/dist",
		"/app/dist",
		"/app/WebX/dist",
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
				return dir
			}
		}
	}
	return ""
}

// isHTMLNavigation checks if a request is a browser HTML page navigation.
func isHTMLNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}

	secDest := strings.ToLower(r.Header.Get("Sec-Fetch-Dest"))
	if secDest == "document" || secDest == "frame" || secDest == "iframe" {
		return true
	}

	secMode := strings.ToLower(r.Header.Get("Sec-Fetch-Mode"))
	if secMode == "navigate" {
		return true
	}

	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "text/html") {
		parts := strings.Split(accept, ",")
		if len(parts) > 0 && !strings.HasPrefix(strings.TrimSpace(parts[0]), "application/json") {
			return true
		}
	}

	return false
}

// isReservedNavigation returns true if HTML navigation should bypass SPA index.html fallback
// (e.g. documentation, audio streams, direct downloads).
func isReservedNavigation(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	if path == "/docs" || strings.HasPrefix(path, "/docs/") ||
		path == "/redoc" || strings.HasPrefix(path, "/redoc/") ||
		path == "/openapi.json" ||
		strings.HasPrefix(path, "/assets/") ||
		strings.HasPrefix(path, "/auth/") ||
		strings.HasPrefix(path, "/covers/") ||
		strings.HasPrefix(path, "/stream/") ||
		strings.HasPrefix(path, "/download/") ||
		(strings.HasPrefix(path, "/tracks/") && (strings.HasSuffix(path, "/stream") || strings.HasSuffix(path, "/download"))) {
		return true
	}
	return false
}

// isAPIOrReservedPath returns true if the URL path belongs to backend API, docs, or media routes.
func isAPIOrReservedPath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	prefixes := []string{
		"/api",
		"/auth",
		"/tracks",
		"/stream",
		"/download",
		"/artists",
		"/albums",
		"/favourites",
		"/playlists",
		"/topics",
		"/history",
		"/me",
		"/listening-events",
		"/daily-playlist",
		"/admin",
		"/share",
		"/recaps",
		"/sources",
		"/discord",
		"/health",
		"/docs",
		"/redoc",
		"/openapi.json",
	}
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// SPAMiddleware intercepts browser HTML page navigations and serves the frontend SPA shell
// (index.html), preserving API endpoints for fetch/XHR calls.
func (s *Server) SPAMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.distDir != "" && isHTMLNavigation(r) {
			path := strings.ToLower(strings.TrimSpace(r.URL.Path))
			if !isReservedNavigation(path) {
				indexFile := filepath.Join(s.distDir, "index.html")
				if _, err := os.Stat(indexFile); err == nil {
					w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
					w.Header().Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
					http.ServeFile(w, r, indexFile)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
