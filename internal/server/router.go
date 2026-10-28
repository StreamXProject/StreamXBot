package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"streamgo/internal/api"
	"streamgo/internal/api/handlers"
	"streamgo/internal/config"
	"streamgo/internal/database"
	"streamgo/internal/logger"
	"streamgo/internal/repository"
	"streamgo/internal/services"
	"streamgo/internal/telegram"
)

var log = logger.New("server")

// Server encapsulates the Chi router, config, and database handle.
type Server struct {
	Router  *chi.Mux
	Config  *config.Config
	DB      *database.Client
	TG      *telegram.Service
	distDir string
	start   time.Time
}

// New creates and configures a new HTTP router with middlewares and modular routes.
func New(cfg *config.Config, db *database.Client, tg *telegram.Service, filter *services.AccessFilter) *Server {
	r := chi.NewRouter()

	distDir := resolveDistDir()
	if distDir != "" {
		log.Infof("[spa] WebX frontend detected and mounted from: %s", distDir)
	}

	s := &Server{
		Router:  r,
		Config:  cfg,
		DB:      db,
		TG:      tg,
		distDir: distDir,
		start:   time.Now(),
	}

	// Global Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	// Toggleable HTTP request logger: completely silent when APILogs is false
	r.Use(logger.HTTPMiddleware(cfg.APILogs))
	r.Use(middleware.Recoverer)

	// CORS Configuration
	corsOrigins := []string{"*"}
	if cfg.CorsOrigin != "" && cfg.CorsOrigin != "*" {
		corsOrigins = []string{cfg.CorsOrigin}
	}

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Range", "X-Secret-Key", "X-Auth-Token"},
		ExposedHeaders:   []string{"Link", "Content-Length", "Content-Range", "Accept-Ranges"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Intercept browser HTML navigations to serve WebX frontend SPA shell
	r.Use(s.SPAMiddleware)

	// Core System Routes
	r.Get("/", s.handleIndex)
	r.Head("/", s.handleIndex)
	r.Get("/health", s.handleHealth)

	// API Documentation Routes (Swagger UI & ReDoc)
	r.Get("/docs", s.handleDocs)
	r.Get("/docs/*", s.handleDocs)
	r.Head("/docs", s.handleDocs)
	r.Get("/redoc", s.handleRedoc)
	r.Head("/redoc", s.handleRedoc)
	r.Get("/openapi.json", s.handleOpenAPISpec)
	r.Head("/openapi.json", s.handleOpenAPISpec)

	// Mount Domain Routes when MongoDB is available
	if db != nil {
		// Repositories
		trackRepo := repository.NewTrackRepository(db)
		artistAlbumRepo := repository.NewArtistAlbumRepository(db)
		favRepo := repository.NewFavouritePlaylistRepository(db)
		userRepo := repository.NewUserRepository(db)
		historyRepo := repository.NewHistoryRepository(db)
		dailyRepo := repository.NewDailyPlaylistRepository(db)
		accessRepo := repository.NewAccessControlRepository(db)
		recapRepo := repository.NewRecapRepository(db)

		// Services
		trackSvc := services.NewTrackService(trackRepo)
		r2Storage := services.NewR2StorageService(cfg)
		trackSvc.SetR2StorageService(r2Storage)
		alacSvc := services.NewALACService(cfg, tg, "stream_media")
		streamSvc := services.NewStreamService(trackRepo, tg)
		streamSvc.SetALACService(alacSvc)
		artistAlbumSvc := services.NewArtistAlbumService(artistAlbumRepo, trackRepo)
		favPlaylistSvc := services.NewFavouritePlaylistService(favRepo, trackRepo, artistAlbumRepo)
		authSvc := services.NewAuthService(cfg, userRepo)
		authSvc.SetAccessControlRepository(accessRepo)
		discordSvc := services.NewDiscordService()
		histSvc := services.NewHistoryService(historyRepo, trackRepo)
		dailySvc := services.NewDailyPlaylistService(dailyRepo, trackRepo)
		accessSvc := services.NewAccessControlService(accessRepo)
		lyricsSvc := services.NewLyricsEnrichmentService(cfg)
		recapSvc := services.NewRecapService(recapRepo, trackRepo)

		// Handlers
		trackHandler := handlers.NewTrackHandler(trackSvc, authSvc, cfg)
		topicHandler := handlers.NewTopicHandler(trackSvc)
		streamHandler := handlers.NewStreamHandler(streamSvc, authSvc, histSvc)
		artistHandler := handlers.NewArtistHandler(artistAlbumSvc)
		albumHandler := handlers.NewAlbumHandler(artistAlbumSvc)
		favHandler := handlers.NewFavouriteHandler(favPlaylistSvc, authSvc)
		playlistHandler := handlers.NewPlaylistHandler(favPlaylistSvc, authSvc)
		authHandler := handlers.NewAuthHandler(cfg, authSvc)
		mediaExtraHandler := handlers.NewMediaExtraHandler(trackSvc, lyricsSvc, trackRepo)
		sourcesHandler := handlers.NewSourcesHandler(cfg, filter, authSvc)
		discordHandler := handlers.NewDiscordHandler(discordSvc)
		historyHandler := handlers.NewHistoryHandler(histSvc, authSvc)
		dailyHandler := handlers.NewDailyPlaylistHandler(dailySvc, authSvc)
		accessHandler := handlers.NewAccessControlHandler(accessSvc, authSvc, cfg)
		shareHandler := handlers.NewShareHandler(trackRepo, favRepo, cfg)
		recapHandler := handlers.NewRecapHandler(recapSvc, authSvc)

		// Register routes
		trackHandler.Routes(r)
		topicHandler.Routes(r)
		streamHandler.Routes(r)
		artistHandler.Routes(r)
		albumHandler.Routes(r)
		favHandler.Routes(r)
		playlistHandler.Routes(r)
		authHandler.Routes(r)
		mediaExtraHandler.Routes(r)
		sourcesHandler.Routes(r)
		discordHandler.Routes(r)
		historyHandler.Routes(r)
		dailyHandler.Routes(r)
		accessHandler.Routes(r)
		shareHandler.Routes(r)
		recapHandler.Routes(r)
	}

	// SPA & Static Asset Catch-All Handler
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if isAPIOrReservedPath(r.URL.Path) {
			api.RespondError(w, http.StatusNotFound, "route not found")
			return
		}

		if s.distDir != "" {
			cleanPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
			if cleanPath != "" && cleanPath != "." {
				targetPath := filepath.Join(s.distDir, cleanPath)
				if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
					if cleanPath == "sw.js" || cleanPath == "manifest.json" {
						w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
					} else if strings.HasPrefix(cleanPath, "assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					http.ServeFile(w, r, targetPath)
					return
				}
			}

			// Single Page Application Navigation: Return index.html for GET/HEAD
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				indexFile := filepath.Join(s.distDir, "index.html")
				if _, err := os.Stat(indexFile); err == nil {
					w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
					w.Header().Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
					http.ServeFile(w, r, indexFile)
					return
				}
			}
		}

		api.RespondError(w, http.StatusNotFound, "route not found")
	})

	return s
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if s.distDir != "" {
		indexFile := filepath.Join(s.distDir, "index.html")
		if _, err := os.Stat(indexFile); err == nil {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
			http.ServeFile(w, r, indexFile)
			return
		}
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"service": "StreamGO",
		"status":  "running",
		"version": "1.0.0",
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbStatus := "disabled"
	if s.DB != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.DB.Ping(ctx); err != nil {
			dbStatus = "unhealthy: " + err.Error()
		} else {
			dbStatus = "healthy"
		}
	}

	tgStatus := "disabled"
	if s.TG != nil {
		tgStatus = s.TG.StatusString()
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"database": dbStatus,
		"telegram": tgStatus,
		"uptime":   time.Since(s.start).String(),
		"time":     time.Now().UTC().Format(time.RFC3339),
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
