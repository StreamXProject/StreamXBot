package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/api/middleware"
	"streamgo/internal/models"
	"streamgo/internal/services"
)

// RecapHandler handles user listening events, recaps, period snapshots, and public share links.
type RecapHandler struct {
	recapSvc *services.RecapService
	authSvc  *services.AuthService
}

// NewRecapHandler creates a new RecapHandler.
func NewRecapHandler(recapSvc *services.RecapService, authSvc *services.AuthService) *RecapHandler {
	return &RecapHandler{
		recapSvc: recapSvc,
		authSvc:  authSvc,
	}
}

// Routes mounts recap and listening event endpoints.
func (h *RecapHandler) Routes(r chi.Router) {
	requireAuth := middleware.RequireAuth(h.authSvc)
	optionalAuth := middleware.OptionalAuth(h.authSvc)

	// Listening events telemetry
	r.With(requireAuth).Post("/me/listening-events", h.RecordListeningEvents)
	r.With(optionalAuth).Post("/listening-events", h.RecordLegacyListeningEvents)

	// Recaps collection and shares management
	r.With(requireAuth).Get("/me/recaps", h.ListRecaps)
	r.With(requireAuth).Get("/me/recaps/shares", h.ListShares)
	r.With(requireAuth).Delete("/me/recaps/shares/{token}", h.RevokeShare)
	r.With(requireAuth).Delete("/me/recaps/data", h.DeleteData)

	// Specific recap snapshots & share generation
	r.With(requireAuth).Get("/me/recaps/{ptype}/{period}", h.GetRecap)
	r.With(requireAuth).Post("/me/recaps/{ptype}/{period}/share", h.ShareRecap)

	// Public share viewer (no auth required)
	r.Get("/recaps/share/{token}", h.GetPublicShare)
}

// RecordListeningEvents handles POST /me/listening-events.
func (h *RecapHandler) RecordListeningEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "user login required")
		return
	}

	var payload models.ListeningEventsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(payload.Events) > 200 {
		api.RespondError(w, http.StatusBadRequest, "batch size cannot exceed 200 events")
		return
	}

	stored, err := h.recapSvc.RecordEvents(r.Context(), userID, payload.Events)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"received": len(payload.Events),
		"stored":   stored,
	})
}

// RecordLegacyListeningEvents handles POST /listening-events.
func (h *RecapHandler) RecordLegacyListeningEvents(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var payload models.ListeningEventsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(payload.Events) == 0 {
		api.RespondJSON(w, http.StatusOK, map[string]any{"ok": true, "count": 0})
		return
	}

	if userID > 0 {
		_, _ = h.recapSvc.RecordEvents(r.Context(), userID, payload.Events)
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"count": len(payload.Events),
	})
}

// ListRecaps handles GET /me/recaps?tz=.
func (h *RecapHandler) ListRecaps(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "user login required")
		return
	}

	tz := api.ParseQueryInt(r, "tz", 0)
	if tz < -840 || tz > 840 {
		api.RespondError(w, http.StatusBadRequest, "tz offset out of range (-840 to 840 minutes)")
		return
	}

	items, err := h.recapSvc.ListAvailable(r.Context(), userID, tz)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*models.RecapAvailableItem{}
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

// ListShares handles GET /me/recaps/shares.
func (h *RecapHandler) ListShares(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "user login required")
		return
	}

	items, err := h.recapSvc.ListShares(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*models.RecapShareItem{}
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

// RevokeShare handles DELETE /me/recaps/shares/{token}.
func (h *RecapHandler) RevokeShare(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "user login required")
		return
	}

	token := chi.URLParam(r, "token")
	if token == "" {
		api.RespondError(w, http.StatusBadRequest, "token is required")
		return
	}

	revoked, err := h.recapSvc.RevokeShare(r.Context(), userID, token)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !revoked {
		api.RespondError(w, http.StatusNotFound, "Share link not found")
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok": true,
	})
}

// DeleteData handles DELETE /me/recaps/data.
func (h *RecapHandler) DeleteData(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "user login required")
		return
	}

	counts, err := h.recapSvc.DeleteUserData(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"events":    counts["events"],
		"snapshots": counts["snapshots"],
		"shares":    counts["shares"],
	})
}

// GetRecap handles GET /me/recaps/{ptype}/{period}?tz=&refresh=.
func (h *RecapHandler) GetRecap(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "user login required")
		return
	}

	ptype := models.RecapPeriodType(chi.URLParam(r, "ptype"))
	if ptype != models.RecapPeriodWeekly && ptype != models.RecapPeriodMonthly && ptype != models.RecapPeriodYearly {
		api.RespondError(w, http.StatusBadRequest, "invalid period type, expected weekly, monthly, or yearly")
		return
	}

	period := chi.URLParam(r, "period")
	if period == "" {
		api.RespondError(w, http.StatusBadRequest, "period parameter is required")
		return
	}

	tz := api.ParseQueryInt(r, "tz", 0)
	if tz < -840 || tz > 840 {
		api.RespondError(w, http.StatusBadRequest, "tz offset out of range (-840 to 840 minutes)")
		return
	}

	refresh := api.ParseQueryBool(r, "refresh", false)

	if !services.IsPeriodAvailable(ptype, period, tz) {
		api.RespondError(w, http.StatusBadRequest, "This recap is not ready yet. Weekly recaps unlock on the weekend (Sunday), monthly on the last day of the month, and yearly on Dec 31st.")
		return
	}

	snap, err := h.recapSvc.GetSnapshot(r.Context(), userID, ptype, period, tz, refresh)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, snap)
}

// ShareRecap handles POST /me/recaps/{ptype}/{period}/share?tz=.
func (h *RecapHandler) ShareRecap(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "user login required")
		return
	}

	ptype := models.RecapPeriodType(chi.URLParam(r, "ptype"))
	if ptype != models.RecapPeriodWeekly && ptype != models.RecapPeriodMonthly && ptype != models.RecapPeriodYearly {
		api.RespondError(w, http.StatusBadRequest, "invalid period type, expected weekly, monthly, or yearly")
		return
	}

	period := chi.URLParam(r, "period")
	if period == "" {
		api.RespondError(w, http.StatusBadRequest, "period parameter is required")
		return
	}

	tz := api.ParseQueryInt(r, "tz", 0)
	if tz < -840 || tz > 840 {
		api.RespondError(w, http.StatusBadRequest, "tz offset out of range (-840 to 840 minutes)")
		return
	}

	if !services.IsPeriodAvailable(ptype, period, tz) {
		api.RespondError(w, http.StatusBadRequest, "This recap is not ready to share yet.")
		return
	}

	snap, err := h.recapSvc.GetSnapshot(r.Context(), userID, ptype, period, tz, false)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if snap.Stats == nil || snap.Stats.TotalPlays == 0 {
		api.RespondError(w, http.StatusBadRequest, "Nothing to share for this period yet")
		return
	}

	shareItem, err := h.recapSvc.CreateShare(r.Context(), userID, snap)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, shareItem)
}

// GetPublicShare handles GET /recaps/share/{token}.
func (h *RecapHandler) GetPublicShare(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		api.RespondError(w, http.StatusNotFound, "This recap link is no longer available")
		return
	}

	summary, err := h.recapSvc.GetPublicShare(r.Context(), token)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if summary == nil {
		api.RespondError(w, http.StatusNotFound, "This recap link is no longer available")
		return
	}

	api.RespondJSON(w, http.StatusOK, summary)
}
