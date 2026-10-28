package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"streamgo/internal/logger"
	"streamgo/internal/models"
)

var logDiscord = logger.New("discord_ra")

const (
	DiscordGatewayURL = "wss://remote-auth-gateway.discord.gg/?v=2"
	DiscordLoginURL   = "https://discord.com/api/v9/users/@me/remote-auth/login"
	SessionTTLSec     = 240
)

// DiscordRemoteAuthSession holds ephemeral state for one QR code login session.
type DiscordRemoteAuthSession struct {
	SessionID string
	PrivKey   *rsa.PrivateKey
	PubKeyB64 string

	Status      string
	Fingerprint string
	QRURL       string
	UserInfo    map[string]any
	Token       string
	Error       string
	CreatedAt   float64

	EventsChan chan map[string]any
	Cancelled  bool
	CancelFunc context.CancelFunc
	mu         sync.RWMutex
}

// DiscordService manages Discord Remote Auth sessions and external assets proxying.
type DiscordService struct {
	sessions map[string]*DiscordRemoteAuthSession
	mu       sync.RWMutex
}

// NewDiscordService creates a DiscordService.
func NewDiscordService() *DiscordService {
	s := &DiscordService{
		sessions: make(map[string]*DiscordRemoteAuthSession),
	}
	go s.cleanupLoop()
	return s
}

func (s *DiscordService) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.cleanupStale()
	}
}

func (s *DiscordService) cleanupStale() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := float64(time.Now().Unix())
	for id, sess := range s.sessions {
		if now-sess.CreatedAt > SessionTTLSec {
			sess.Cancel()
			delete(s.sessions, id)
		}
	}
}

// StartSession creates a new Remote Auth session and kicks off the gateway listener in background.
func (s *DiscordService) StartSession(ctx context.Context) (*models.StartRemoteAuthResponse, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	spkiDER, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	pubKeyB64 := base64.StdEncoding.EncodeToString(spkiDER)

	idBytes := make([]byte, 16)
	_, _ = rand.Read(idBytes)
	sessionID := fmt.Sprintf("%x-%x-%x-%x-%x", idBytes[0:4], idBytes[4:6], idBytes[6:8], idBytes[8:10], idBytes[10:16])

	sessCtx, cancel := context.WithCancel(context.Background())
	sess := &DiscordRemoteAuthSession{
		SessionID:  sessionID,
		PrivKey:    privKey,
		PubKeyB64:  pubKeyB64,
		Status:     "init",
		CreatedAt:  float64(time.Now().Unix()),
		EventsChan: make(chan map[string]any, 64),
		CancelFunc: cancel,
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	go sess.Run(sessCtx)

	// Wait up to 4 seconds for initial QR fingerprint to appear
	timeout := time.After(4 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return &models.StartRemoteAuthResponse{
				SessionID: sessionID,
				Status:    sess.GetStatus(),
			}, nil
		case <-ticker.C:
			sess.mu.RLock()
			fp := sess.Fingerprint
			qr := sess.QRURL
			status := sess.Status
			errMsg := sess.Error
			sess.mu.RUnlock()

			if errMsg != "" {
				return nil, errors.New(errMsg)
			}
			if fp != "" {
				return &models.StartRemoteAuthResponse{
					SessionID:   sessionID,
					Status:      status,
					URL:         qr,
					Fingerprint: fp,
				}, nil
			}
		}
	}
}

// CreateWSSession starts a remote auth session for direct WebSocket streaming.
func (s *DiscordService) CreateWSSession(ctx context.Context) (*DiscordRemoteAuthSession, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	spkiDER, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	pubKeyB64 := base64.StdEncoding.EncodeToString(spkiDER)

	idBytes := make([]byte, 16)
	_, _ = rand.Read(idBytes)
	sessionID := fmt.Sprintf("%x-%x-%x-%x-%x", idBytes[0:4], idBytes[4:6], idBytes[6:8], idBytes[8:10], idBytes[10:16])

	sessCtx, cancel := context.WithCancel(ctx)
	sess := &DiscordRemoteAuthSession{
		SessionID:  sessionID,
		PrivKey:    privKey,
		PubKeyB64:  pubKeyB64,
		Status:     "init",
		CreatedAt:  float64(time.Now().Unix()),
		EventsChan: make(chan map[string]any, 64),
		CancelFunc: cancel,
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	go sess.Run(sessCtx)
	return sess, nil
}

// GetStatus returns the current status and user/token if completed.
func (s *DiscordService) GetStatus(sessionID string) (*models.RemoteAuthStatusResponse, error) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("session not found or expired")
	}

	sess.mu.RLock()
	defer sess.mu.RUnlock()

	return &models.RemoteAuthStatusResponse{
		SessionID:   sess.SessionID,
		Status:      sess.Status,
		URL:         sess.QRURL,
		Fingerprint: sess.Fingerprint,
		User:        sess.UserInfo,
		Token:       sess.Token,
		Error:       sess.Error,
	}, nil
}

// CancelSession terminates an active session.
func (s *DiscordService) CancelSession(sessionID string) error {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if ok {
		delete(s.sessions, sessionID)
	}
	s.mu.Unlock()

	if ok && sess != nil {
		sess.Cancel()
	}
	return nil
}

// GetSession retrieves session by ID.
func (s *DiscordService) GetSession(sessionID string) *DiscordRemoteAuthSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

// Cancel terminates the session context.
func (sess *DiscordRemoteAuthSession) Cancel() {
	sess.mu.Lock()
	sess.Cancelled = true
	sess.Status = "cancelled"
	sess.mu.Unlock()
	if sess.CancelFunc != nil {
		sess.CancelFunc()
	}
}

// GetStatus gets status safely.
func (sess *DiscordRemoteAuthSession) GetStatus() string {
	sess.mu.RLock()
	defer sess.mu.RUnlock()
	return sess.Status
}

func (sess *DiscordRemoteAuthSession) decryptOAEP(encB64 string) ([]byte, error) {
	cipherBytes, err := base64.StdEncoding.DecodeString(encB64)
	if err != nil {
		return nil, err
	}
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, sess.PrivKey, cipherBytes, nil)
}

func (sess *DiscordRemoteAuthSession) emit(event map[string]any) {
	select {
	case sess.EventsChan <- event:
	default:
	}
}

// Run manages the WebSocket connection to Discord's Remote Auth Gateway.
func (sess *DiscordRemoteAuthSession) Run(ctx context.Context) {
	defer func() {
		close(sess.EventsChan)
	}()

	header := make(http.Header)
	header.Set("Origin", "https://discord.com")
	header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")

	conn, _, err := websocket.Dial(ctx, DiscordGatewayURL, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		sess.mu.Lock()
		sess.Error = err.Error()
		sess.Status = "error"
		sess.mu.Unlock()
		sess.emit(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	sess.emit(map[string]any{"type": "status", "status": "connecting", "session_id": sess.SessionID})

	var heartbeatCancel context.CancelFunc

	for {
		select {
		case <-ctx.Done():
			if heartbeatCancel != nil {
				heartbeatCancel()
			}
			return
		default:
		}

		_, data, err := conn.Read(ctx)
		if err != nil {
			if !sess.Cancelled {
				sess.mu.Lock()
				sess.Status = "error"
				sess.Error = err.Error()
				sess.mu.Unlock()
				sess.emit(map[string]any{"type": "error", "message": err.Error()})
			}
			if heartbeatCancel != nil {
				heartbeatCancel()
			}
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			continue
		}

		op, _ := payload["op"].(string)

		switch op {
		case "hello":
			intervalMs := 41250.0
			if v, ok := payload["heartbeat_interval"].(float64); ok && v > 0 {
				intervalMs = v
			}
			hbCtx, hbCancel := context.WithCancel(ctx)
			heartbeatCancel = hbCancel
			go sess.heartbeatLoop(hbCtx, conn, intervalMs)

			initMsg := map[string]any{
				"op":                 "init",
				"encoded_public_key": sess.PubKeyB64,
			}
			initBytes, _ := json.Marshal(initMsg)
			_ = conn.Write(ctx, websocket.MessageText, initBytes)

		case "nonce_proof":
			encNonce, _ := payload["encrypted_nonce"].(string)
			decrypted, err := sess.decryptOAEP(encNonce)
			if err != nil {
				sess.mu.Lock()
				sess.Status = "error"
				sess.Error = "nonce proof failed: " + err.Error()
				sess.mu.Unlock()
				sess.emit(map[string]any{"type": "error", "message": sess.Error})
				return
			}
			digest := sha256.Sum256(decrypted)
			proof := base64.RawURLEncoding.EncodeToString(digest[:])
			proofMsg := map[string]any{
				"op":    "nonce_proof",
				"proof": proof,
			}
			proofBytes, _ := json.Marshal(proofMsg)
			_ = conn.Write(ctx, websocket.MessageText, proofBytes)

		case "fingerprint", "pending_remote_init":
			fp, _ := payload["fingerprint"].(string)
			if fp != "" && sess.Fingerprint == "" {
				sess.mu.Lock()
				sess.Fingerprint = fp
				sess.QRURL = fmt.Sprintf("https://discord.com/ra/%s", fp)
				sess.Status = "waiting_scan"
				sess.mu.Unlock()

				sess.emit(map[string]any{
					"type":        "qr",
					"url":         sess.QRURL,
					"fingerprint": fp,
					"session_id":  sess.SessionID,
				})
			}

			if userMap, ok := payload["user"].(map[string]any); ok && userMap != nil {
				sess.mu.Lock()
				sess.UserInfo = userMap
				sess.Status = "scanned"
				sess.mu.Unlock()

				sess.emit(map[string]any{
					"type":       "scanned",
					"user":       userMap,
					"session_id": sess.SessionID,
				})
			}

		case "pending_ticket":
			encUser, _ := payload["encrypted_user_payload"].(string)
			if encUser != "" {
				if decUser, err := sess.decryptOAEP(encUser); err == nil {
					parts := strings.Split(string(decUser), ":")
					uID := ""
					uDisc := "0"
					uAv := ""
					uName := "Discord User"
					if len(parts) > 0 {
						uID = parts[0]
					}
					if len(parts) > 1 {
						uDisc = parts[1]
					}
					if len(parts) > 2 && parts[2] != "None" {
						uAv = parts[2]
					}
					if len(parts) > 3 {
						uName = parts[3]
					}
					sess.mu.Lock()
					sess.UserInfo = map[string]any{
						"id":            uID,
						"username":      uName,
						"discriminator": uDisc,
						"avatar":        uAv,
					}
					sess.Status = "scanned"
					sess.mu.Unlock()
				}
			}
			sess.emit(map[string]any{
				"type":       "scanned",
				"user":       sess.UserInfo,
				"session_id": sess.SessionID,
			})

		case "pending_login":
			ticket, _ := payload["ticket"].(string)
			if ticket == "" {
				continue
			}

			sess.mu.Lock()
			sess.Status = "confirming"
			sess.mu.Unlock()
			sess.emit(map[string]any{"type": "status", "status": "confirming", "session_id": sess.SessionID})

			// Exchange ticket with Discord REST API
			token, err := sess.exchangeTicket(ctx, ticket)
			if err != nil {
				sess.mu.Lock()
				sess.Status = "error"
				sess.Error = err.Error()
				sess.mu.Unlock()
				sess.emit(map[string]any{"type": "error", "message": err.Error()})
				return
			}

			sess.mu.Lock()
			sess.Token = token
			sess.Status = "success"
			sess.mu.Unlock()

			sess.emit(map[string]any{
				"type":       "success",
				"token":      token,
				"user":       sess.UserInfo,
				"session_id": sess.SessionID,
			})

			// Graceful cancel after completion
			cancelMsg, _ := json.Marshal(map[string]any{"op": "cancel"})
			_ = conn.Write(ctx, websocket.MessageText, cancelMsg)
			return

		case "cancel":
			sess.mu.Lock()
			sess.Status = "cancelled"
			sess.mu.Unlock()
			sess.emit(map[string]any{"type": "cancelled", "session_id": sess.SessionID})
			return
		}
	}
}

func (sess *DiscordRemoteAuthSession) heartbeatLoop(ctx context.Context, conn *websocket.Conn, intervalMs float64) {
	intervalSec := (intervalMs / 1000.0) * 0.9
	if intervalSec < 5.0 {
		intervalSec = 5.0
	}
	ticker := time.NewTicker(time.Duration(intervalSec * float64(time.Second)))
	defer ticker.Stop()

	hbMsg, _ := json.Marshal(map[string]any{"op": "heartbeat"})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.Write(ctx, websocket.MessageText, hbMsg); err != nil {
				return
			}
		}
	}
}

func (sess *DiscordRemoteAuthSession) exchangeTicket(ctx context.Context, ticket string) (string, error) {
	body, _ := json.Marshal(map[string]any{"ticket": ticket})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DiscordLoginURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ticket exchange returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var data struct {
		EncryptedToken string `json:"encrypted_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	if data.EncryptedToken == "" {
		return "", errors.New("missing encrypted_token in response")
	}

	decryptedToken, err := sess.decryptOAEP(data.EncryptedToken)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt user token: %w", err)
	}

	return string(decryptedToken), nil
}

// ResolveExternalAssets proxies external asset requests to Discord API.
func (s *DiscordService) ResolveExternalAssets(ctx context.Context, req models.ExternalAssetsRequest) ([]map[string]any, error) {
	appID := req.ApplicationID
	if appID == "" {
		appID = "1547543416143876167"
	}
	token := strings.TrimSpace(req.Token)
	if token == "" || len(req.URLs) == 0 {
		return []map[string]any{}, nil
	}

	discordURL := fmt.Sprintf("https://discord.com/api/v9/applications/%s/external-assets", appID)
	body, _ := json.Marshal(map[string]any{"urls": req.URLs})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, discordURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return []map[string]any{}, nil
	}

	var results []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}
	return results, nil
}
