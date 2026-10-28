package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/transport"

	"streamgo/internal/config"
	"streamgo/internal/logger"
	"streamgo/internal/models"
)

var log = logger.New("telegram")

// ClientWorker represents a single authenticated MTProto client connection.
type ClientWorker struct {
	ID                  int64
	FirstName           string
	Username            string
	Bot                 bool
	Token               string
	Client              *telegram.Client
	API                 *tg.Client
	Downloader          *downloader.Downloader
	Workload            int64
	Ready               bool
	channelAccessHashes map[int64]int64
	channelAccessMu     sync.RWMutex
	userAccessHashes    map[int64]int64
	userAccessMu        sync.RWMutex
}

// Service manages a pool of concurrent MTProto Telegram bot clients.
type Service struct {
	Config        *config.Config
	Dispatcher    tg.UpdateDispatcher
	primaryWorker *ClientWorker
	workers       []*ClientWorker
	mu            sync.RWMutex
	stopCancel    context.CancelFunc
}

// New creates an unstarted Telegram multi-client pool.
func New(cfg *config.Config) (*Service, error) {
	if cfg.ApiID <= 0 || cfg.ApiHash == "" {
		return nil, errors.New("API_ID and API_HASH are required to initialize Telegram client")
	}

	dispatcher := tg.NewUpdateDispatcher()

	svc := &Service{
		Config:     cfg,
		workers:    make([]*ClientWorker, 0),
		Dispatcher: dispatcher,
	}

	sessionDir := "stream_media/sessions"
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		// Directory may be owned by root (Docker); fall back to a writable temp location
		sessionDir = filepath.Join(os.TempDir(), "streamgo_sessions")
		_ = os.MkdirAll(sessionDir, 0700)
		log.Warnf("Default session directory not writable, using fallback: %s", sessionDir)
	} else {
		// Verify the directory is actually writable by trying to create a temp file
		testFile := filepath.Join(sessionDir, ".write_test")
		if f, err := os.Create(testFile); err != nil {
			sessionDir = filepath.Join(os.TempDir(), "streamgo_sessions")
			_ = os.MkdirAll(sessionDir, 0700)
			log.Warnf("Session directory exists but not writable, using fallback: %s", sessionDir)
		} else {
			f.Close()
			os.Remove(testFile)
		}
	}

	createOptions := func(sessionPath string, noUpdates bool) telegram.Options {
		opts := telegram.Options{
			NoUpdates:        noUpdates,
			SessionStorage:   &session.FileStorage{Path: sessionPath},
			MigrationTimeout: 60 * time.Second, // Allow cloud environments ample time for DC migration
			DialTimeout:      20 * time.Second,
			ExchangeTimeout:  30 * time.Second,
			Resolver: dcs.Plain(dcs.PlainOptions{
				Protocol:   transport.Abridged,
				Obfuscated: true,
				Network:    "tcp4", // Force IPv4 to avoid hanging on unroutable IPv6 cloud networks (e.g. Render)
			}),
		}
		if !noUpdates {
			opts.UpdateHandler = dispatcher
		}
		return opts
	}

	// 1. Create Primary Worker with persistent session storage
	primaryTokenPrefix := strings.Split(cfg.BotToken, ":")[0]
	primarySessionPath := filepath.Join(sessionDir, fmt.Sprintf("session_%s.json", primaryTokenPrefix))

	primaryClient := telegram.NewClient(cfg.ApiID, cfg.ApiHash, createOptions(primarySessionPath, false))
	svc.primaryWorker = &ClientWorker{
		Token:               cfg.BotToken,
		Client:              primaryClient,
		API:                 primaryClient.API(),
		Downloader:          downloader.NewDownloader(),
		channelAccessHashes: make(map[int64]int64),
		userAccessHashes:    make(map[int64]int64),
	}
	svc.workers = append(svc.workers, svc.primaryWorker)

	// 2. Create Secondary Multi-Client Workers with persistent session storage
	if cfg.MultiClients {
		for i, tok := range cfg.MultiClientTokens {
			tok = strings.TrimSpace(tok)
			if tok == "" || tok == cfg.BotToken {
				continue
			}
			tokenPrefix := strings.Split(tok, ":")[0]
			sessionPath := filepath.Join(sessionDir, fmt.Sprintf("session_%s.json", tokenPrefix))

			workerClient := telegram.NewClient(cfg.ApiID, cfg.ApiHash, createOptions(sessionPath, true))
			worker := &ClientWorker{
				Token:               tok,
				Client:              workerClient,
				API:                 workerClient.API(),
				Downloader:          downloader.NewDownloader(),
				channelAccessHashes: make(map[int64]int64),
				userAccessHashes:    make(map[int64]int64),
			}
			svc.workers = append(svc.workers, worker)
			log.Infof("Registered multi-client worker #%d", i+1)
		}
	}

	return svc, nil
}

// Start launches all MTProto client workers concurrently with automatic reconnection.
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.stopCancel = cancel

	readyChan := make(chan struct{})
	var once sync.Once

	for _, w := range s.workers {
		worker := w
		go func() {
			backoffDelay := 2 * time.Second
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				err := worker.Client.Run(ctx, func(runCtx context.Context) error {
					// Authenticate
					authStatus, err := worker.Client.Auth().Status(runCtx)
					if err != nil {
						log.Warnf("Worker check auth failed: %v", err)
						return err
					}

					if !authStatus.Authorized && worker.Token != "" {
						log.Infof("Authorizing client with token (prefix: %s...)", worker.Token[:min(10, len(worker.Token))])
						if _, err := worker.Client.Auth().Bot(runCtx, worker.Token); err != nil {
							log.Warnf("Worker bot auth failed: %v", err)
							return err
						}
						log.Info("Worker authentication successful!")
					}

					self, err := worker.Client.Self(runCtx)
					if err == nil && self != nil {
						s.mu.Lock()
						worker.ID = self.ID
						worker.FirstName = self.FirstName
						worker.Username = self.Username
						worker.Bot = self.Bot
						worker.Ready = true
						s.mu.Unlock()
						log.Infof("Connected client: %s (@%s, ID: %d, Bot: %v)", self.FirstName, self.Username, self.ID, self.Bot)
					}

					// Reset backoff upon successful connection and authorization
					backoffDelay = 2 * time.Second

					if worker == s.primaryWorker {
						once.Do(func() {
							close(readyChan)
						})
					}

					<-runCtx.Done()
					return runCtx.Err()
				})

				s.mu.Lock()
				worker.Ready = false
				s.mu.Unlock()

				if ctx.Err() != nil {
					return
				}

				if err != nil {
					log.Warnf("Telegram worker (token prefix: %s) disconnected: %v. Reconnecting in %v...",
						worker.Token[:min(10, len(worker.Token))], err, backoffDelay)
				}

				select {
				case <-ctx.Done():
					return
				case <-time.After(backoffDelay):
				}

				if backoffDelay < 30*time.Second {
					backoffDelay *= 2
				}
			}
		}()
	}

	// Wait up to 45 seconds for primary client readiness
	select {
	case <-readyChan:
		log.Infof("Telegram service active with %d client(s) ready", s.ReadyWorkerCount())
		return nil
	case <-time.After(45 * time.Second):
		log.Warn("Telegram service started with delayed initialization")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReadyWorkerCount returns number of connected and ready workers.
func (s *Service) ReadyWorkerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, w := range s.workers {
		if w.Ready {
			count++
		}
	}
	return count
}

// PrimaryWorker returns the main bot worker.
func (s *Service) PrimaryWorker() *ClientWorker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.primaryWorker
}

// AcquireWorker selects the best ready worker with the lowest workload (load-balancing).
func (s *Service) AcquireWorker() *ClientWorker {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var best *ClientWorker
	minLoad := int64(1<<62 - 1)

	for _, w := range s.workers {
		if !w.Ready {
			continue
		}
		load := atomic.LoadInt64(&w.Workload)
		if best == nil || load < minLoad {
			best = w
			minLoad = load
		}
	}

	if best == nil {
		best = s.primaryWorker
	}

	if best != nil {
		atomic.AddInt64(&best.Workload, 1)
	}
	return best
}

// AcquireWorkerForBot attempts to acquire a specific worker by Telegram bot user ID.
func (s *Service) AcquireWorkerForBot(botID string) *ClientWorker {
	botID = strings.TrimSpace(botID)
	if botID != "" {
		s.mu.RLock()
		for _, w := range s.workers {
			if w.Ready && strconv.FormatInt(w.ID, 10) == botID {
				s.mu.RUnlock()
				atomic.AddInt64(&w.Workload, 1)
				return w
			}
		}
		s.mu.RUnlock()
	}
	return s.AcquireWorker()
}

// ReleaseWorker decrements active download workload for a worker.
func (s *Service) ReleaseWorker(w *ClientWorker) {
	if w != nil {
		atomic.AddInt64(&w.Workload, -1)
	}
}

// Stop gracefully stops all Telegram client workers.
func (s *Service) Stop() {
	if s.stopCancel != nil {
		s.stopCancel()
	}
}

// IsReady returns true if at least the primary client is connected.
func (s *Service) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.primaryWorker != nil && s.primaryWorker.Ready
}

// Workers returns a snapshot slice of all registered ClientWorkers.
func (s *Service) Workers() []*ClientWorker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*ClientWorker, len(s.workers))
	copy(res, s.workers)
	return res
}

// Self returns primary bot user info.
func (s *Service) Self() *tg.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.primaryWorker != nil && s.primaryWorker.Ready {
		return &tg.User{
			ID:        s.primaryWorker.ID,
			FirstName: s.primaryWorker.FirstName,
			Username:  s.primaryWorker.Username,
			Bot:       s.primaryWorker.Bot,
		}
	}
	return nil
}

// StatusString returns summary of connected bot clients for /health.
func (s *Service) StatusString() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	readyWorkers := 0
	var names []string
	for _, w := range s.workers {
		if w.Ready {
			readyWorkers++
			if w.Username != "" {
				names = append(names, fmt.Sprintf("@%s", w.Username))
			} else if w.FirstName != "" {
				names = append(names, w.FirstName)
			}
		}
	}

	if readyWorkers == 0 {
		return "connecting"
	}
	return fmt.Sprintf("connected (%s)", strings.Join(names, ", "))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type partialWriter struct {
	w      io.Writer
	remain int64
}

func (pw *partialWriter) Write(p []byte) (int, error) {
	if pw.remain <= 0 {
		return 0, io.EOF
	}

	toWrite := p
	if int64(len(toWrite)) > pw.remain {
		toWrite = toWrite[:pw.remain]
	}

	n, err := pw.w.Write(toWrite)
	pw.remain -= int64(n)
	if err != nil {
		return n, err
	}
	if pw.remain <= 0 {
		return n, io.EOF
	}
	return n, nil
}

// DownloadPartial streams up to maxBytes of a document location into w (or entire document if maxBytes <= 0).
func (s *Service) DownloadPartial(ctx context.Context, location *tg.InputDocumentFileLocation, maxBytes int64, w io.Writer) error {
	worker := s.AcquireWorker()
	defer s.ReleaseWorker(worker)

	var targetWriter io.Writer = w
	if maxBytes > 0 {
		targetWriter = &partialWriter{
			w:      w,
			remain: maxBytes,
		}
	}

	downloader := worker.Client.Downloader()
	_, err := downloader.Download(worker.API, location).Stream(ctx, targetWriter)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// DownloadPartialByFileID decodes a Pyrogram/TDLib file_id and downloads up to maxBytes.
func (s *Service) DownloadPartialByFileID(ctx context.Context, fileID string, maxBytes int64, w io.Writer) error {
	decoded, err := DecodeFileID(fileID)
	if err != nil {
		return fmt.Errorf("failed to decode file_id: %w", err)
	}

	location := &tg.InputDocumentFileLocation{
		ID:            decoded.MediaID,
		AccessHash:    decoded.AccessHash,
		FileReference: decoded.FileReference,
	}

	return s.DownloadPartial(ctx, location, maxBytes, w)
}

func normalizeToChannelID(chatID int64) (int64, bool) {
	if chatID <= -1000000000000 {
		return -chatID - 1000000000000, true
	}
	if chatID > 1000000000 {
		return chatID, true
	}
	return chatID, false
}

func (w *ClientWorker) GetChannelAccessHash(channelID int64) int64 {
	w.channelAccessMu.RLock()
	defer w.channelAccessMu.RUnlock()
	if w.channelAccessHashes == nil {
		return 0
	}
	return w.channelAccessHashes[channelID]
}

func (w *ClientWorker) getChannelAccessHash(channelID int64) int64 {
	return w.GetChannelAccessHash(channelID)
}

func (w *ClientWorker) SetChannelAccessHash(channelID, accessHash int64) {
	w.setChannelAccessHash(channelID, accessHash)
}

func (w *ClientWorker) setChannelAccessHash(channelID, accessHash int64) {
	w.channelAccessMu.Lock()
	defer w.channelAccessMu.Unlock()
	if w.channelAccessHashes == nil {
		w.channelAccessHashes = make(map[int64]int64)
	}
	w.channelAccessHashes[channelID] = accessHash
}

func (w *ClientWorker) GetUserAccessHash(userID int64) int64 {
	w.userAccessMu.RLock()
	defer w.userAccessMu.RUnlock()
	if w.userAccessHashes == nil {
		return 0
	}
	return w.userAccessHashes[userID]
}

func (w *ClientWorker) SetUserAccessHash(userID, accessHash int64) {
	w.userAccessMu.Lock()
	defer w.userAccessMu.Unlock()
	if w.userAccessHashes == nil {
		w.userAccessHashes = make(map[int64]int64)
	}
	w.userAccessHashes[userID] = accessHash
}

func extractMessagesList(msgs tg.MessagesMessagesClass) []tg.MessageClass {
	if msgs == nil {
		return nil
	}
	switch m := msgs.(type) {
	case *tg.MessagesChannelMessages:
		return m.Messages
	case *tg.MessagesMessages:
		return m.Messages
	case *tg.MessagesMessagesSlice:
		return m.Messages
	default:
		return nil
	}
}

// FetchFileIDForMessage fetches a message from chatID/msgID and generates this worker's distinct file_id.
func (w *ClientWorker) FetchFileIDForMessage(ctx context.Context, chatID int64, msgID int32) (string, error) {
	if !w.Ready || w.API == nil {
		return "", errors.New("worker is not ready")
	}

	channelID, isChannel := normalizeToChannelID(chatID)
	if isChannel {
		accessHash := w.getChannelAccessHash(channelID)
		if accessHash == 0 {
			// Try MessagesGetChats first (uses raw int64 channel IDs without needing prior access hash)
			chats, err := w.API.MessagesGetChats(ctx, []int64{channelID})
			if err == nil {
				for _, chat := range chats.GetChats() {
					if ch, ok := chat.(*tg.Channel); ok && ch.ID == channelID {
						accessHash = ch.AccessHash
						w.setChannelAccessHash(channelID, accessHash)
						break
					}
				}
			}
			if accessHash == 0 {
				channels, err := w.API.ChannelsGetChannels(ctx, []tg.InputChannelClass{
					&tg.InputChannel{ChannelID: channelID},
				})
				if err == nil {
					for _, chat := range channels.GetChats() {
						if ch, ok := chat.(*tg.Channel); ok && ch.ID == channelID {
							accessHash = ch.AccessHash
							w.setChannelAccessHash(channelID, accessHash)
							break
						}
					}
				}
			}
		}

		req := &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{
				ChannelID:  channelID,
				AccessHash: accessHash,
			},
			ID: []tg.InputMessageClass{
				&tg.InputMessageID{ID: int(msgID)},
			},
		}

		msgs, err := w.API.ChannelsGetMessages(ctx, req)
		if err != nil {
			return "", fmt.Errorf("channels get messages: %w", err)
		}

		for _, m := range extractMessagesList(msgs) {
			if msg, ok := m.(*tg.Message); ok {
				if mediaDoc, ok := msg.Media.(*tg.MessageMediaDocument); ok {
					if doc, ok := mediaDoc.Document.(*tg.Document); ok {
						return EncodeFileID(doc.ID, doc.AccessHash, int32(doc.DCID), doc.FileReference), nil
					}
				}
			}
		}
		return "", errors.New("no document found in channel message")
	}

	// Non-channel chats
	msgs, err := w.API.MessagesGetMessages(ctx, []tg.InputMessageClass{
		&tg.InputMessageID{ID: int(msgID)},
	})
	if err != nil {
		return "", fmt.Errorf("messages get messages: %w", err)
	}

	for _, m := range extractMessagesList(msgs) {
		if msg, ok := m.(*tg.Message); ok {
			if mediaDoc, ok := msg.Media.(*tg.MessageMediaDocument); ok {
				if doc, ok := mediaDoc.Document.(*tg.Document); ok {
					return EncodeFileID(doc.ID, doc.AccessHash, int32(doc.DCID), doc.FileReference), nil
				}
			}
		}
	}
	return "", errors.New("no document found in message")
}

// SyncFileIDs collects file_ids for all ready workers for a given message.
func (s *Service) SyncFileIDs(ctx context.Context, chatID int64, msgID int32, fallbackChatID int64, fallbackMsgID int32) map[string]string {
	workers := s.Workers()
	if len(workers) == 0 {
		return map[string]string{}
	}

	type syncResult struct {
		botID  string
		fileID string
	}

	results := make(chan syncResult, len(workers))
	var wg sync.WaitGroup

	for _, w := range workers {
		if !w.Ready {
			continue
		}
		worker := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			botID := strconv.FormatInt(worker.ID, 10)
			if botID == "0" || botID == "" {
				if parts := strings.Split(worker.Token, ":"); len(parts) > 0 {
					botID = parts[0]
				}
			}
			if botID == "0" || botID == "" {
				return
			}

			// Try primary chat ID & msg ID first
			fid, err := worker.FetchFileIDForMessage(ctx, chatID, msgID)
			if (err != nil || fid == "") && fallbackChatID != 0 && fallbackMsgID != 0 {
				fid, err = worker.FetchFileIDForMessage(ctx, fallbackChatID, fallbackMsgID)
			}

			if err == nil && fid != "" {
				results <- syncResult{botID: botID, fileID: fid}
			}
		}()
	}

	wg.Wait()
	close(results)

	res := make(map[string]string)
	for r := range results {
		res[r.botID] = r.fileID
	}
	return res
}

// DownloadPartialForTrack resiliently downloads a chunk of the track, matching workers and auto-refreshing expired file references.
func (s *Service) DownloadPartialForTrack(
	ctx context.Context,
	track *models.Track,
	maxBytes int64,
	w io.Writer,
	onRefreshed func(botID, fileID string),
) error {
	if track == nil {
		return errors.New("track is nil")
	}

	readyWorkers := []*ClientWorker{}
	s.mu.RLock()
	for _, worker := range s.workers {
		if worker.Ready {
			readyWorkers = append(readyWorkers, worker)
		}
	}
	s.mu.RUnlock()

	if len(readyWorkers) == 0 {
		return errors.New("no ready telegram workers available")
	}

	var chosenWorker *ClientWorker
	var fileID string

	// 1. Prefer a worker that already has a mapped file_id
	if track.Telegram.FileIDs != nil {
		for _, worker := range readyWorkers {
			wid := strconv.FormatInt(worker.ID, 10)
			if wid == "0" || wid == "" {
				if parts := strings.Split(worker.Token, ":"); len(parts) > 0 {
					wid = parts[0]
				}
			}
			if fid, ok := track.Telegram.FileIDs[wid]; ok && fid != "" {
				chosenWorker = worker
				fileID = fid
				atomic.AddInt64(&worker.Workload, 1)
				break
			}
		}
	}

	// 2. If not matched, pick the least-loaded worker
	if chosenWorker == nil {
		chosenWorker = s.AcquireWorker()
	}
	defer s.ReleaseWorker(chosenWorker)

	wid := strconv.FormatInt(chosenWorker.ID, 10)
	if wid == "0" || wid == "" {
		if parts := strings.Split(chosenWorker.Token, ":"); len(parts) > 0 {
			wid = parts[0]
		}
	}

	if fileID == "" && track.Telegram.FileIDs != nil {
		fileID = track.Telegram.FileIDs[wid]
	}

	fetchFresh := func() (string, error) {
		// Try cache channel first
		if track.CacheChatID != 0 && track.CacheMessageID != 0 {
			f, err := chosenWorker.FetchFileIDForMessage(ctx, track.CacheChatID, track.CacheMessageID)
			if err == nil && f != "" {
				return f, nil
			}
		}
		// Fallback to source chat
		if track.SourceChatID != 0 && track.SourceMessageID != 0 {
			f, err := chosenWorker.FetchFileIDForMessage(ctx, track.SourceChatID, track.SourceMessageID)
			if err == nil && f != "" {
				return f, nil
			}
		}
		return "", errors.New("could not resolve message to fetch fresh file_id")
	}

	if fileID == "" {
		if fresh, err := fetchFresh(); err == nil && fresh != "" {
			fileID = fresh
			if onRefreshed != nil {
				onRefreshed(wid, fileID)
			}
		} else {
			fileID = track.Telegram.FileID
		}
	}

	if fileID == "" {
		return errors.New("no file_id available for track")
	}

	doDownload := func(fid string) error {
		decoded, err := DecodeFileID(fid)
		if err != nil {
			return fmt.Errorf("failed to decode file_id: %w", err)
		}
		location := &tg.InputDocumentFileLocation{
			ID:            decoded.MediaID,
			AccessHash:    decoded.AccessHash,
			FileReference: decoded.FileReference,
		}

		var targetWriter io.Writer = w
		if maxBytes > 0 {
			targetWriter = &partialWriter{
				w:      w,
				remain: maxBytes,
			}
		}
		dl := chosenWorker.Client.Downloader().WithPartSize(512 * 1024)
		builder := dl.Download(chosenWorker.API, location)
		if maxBytes <= 0 {
			if wa, ok := w.(io.WriterAt); ok {
				_, dlErr := builder.WithThreads(4).Parallel(ctx, wa)
				if dlErr != nil && !errors.Is(dlErr, io.EOF) && !errors.Is(dlErr, context.Canceled) {
					return dlErr
				}
				return nil
			}
		}
		_, dlErr := builder.Stream(ctx, targetWriter)
		if dlErr != nil && !errors.Is(dlErr, io.EOF) && !errors.Is(dlErr, context.Canceled) {
			return dlErr
		}
		return nil
	}

	err := doDownload(fileID)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "FILE_REFERENCE") || strings.Contains(errStr, "400") {
			log.Warnf("File reference expired for track %s on worker %s; refreshing from cache channel...", track.ID, wid)
			if fresh, refreshErr := fetchFresh(); refreshErr == nil && fresh != "" {
				fileID = fresh
				if onRefreshed != nil {
					onRefreshed(wid, fileID)
				}
				// Reset writer if seeker
				if seeker, ok := w.(io.WriteSeeker); ok {
					_, _ = seeker.Seek(0, io.SeekStart)
				}
				if trunc, ok := w.(interface{ Truncate(int64) error }); ok {
					_ = trunc.Truncate(0)
				}
				err = doDownload(fileID)
			}
		}
	}

	return err
}

// DownloadTrack downloads the entire track media into w without byte truncation.
func (s *Service) DownloadTrack(ctx context.Context, track *models.Track, w io.Writer) error {
	return s.DownloadPartialForTrack(ctx, track, 0, w, nil)
}

// isImageBytes verifies if payload starts with valid JPEG, PNG, or WebP magic headers.
func isImageBytes(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// JPEG: 0xFF 0xD8 0xFF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true
	}
	// PNG: 0x89 'P' 'N' 'G'
	if data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return true
	}
	// WebP: RIFF....WEBP
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return true
	}
	return false
}

// DownloadDocumentThumbnailForTrack downloads the standalone Telegram MTProto thumbnail for a track, if available.
func (s *Service) DownloadDocumentThumbnailForTrack(
	ctx context.Context,
	track *models.Track,
	w io.Writer,
) error {
	if track == nil {
		return errors.New("track is nil")
	}

	readyWorkers := []*ClientWorker{}
	s.mu.RLock()
	for _, worker := range s.workers {
		if worker.Ready {
			readyWorkers = append(readyWorkers, worker)
		}
	}
	s.mu.RUnlock()

	if len(readyWorkers) == 0 {
		return errors.New("no ready telegram workers available")
	}

	var chosenWorker *ClientWorker
	var fileID string

	// 1. Prefer a worker that already has a mapped file_id
	if track.Telegram.FileIDs != nil {
		for _, worker := range readyWorkers {
			wid := strconv.FormatInt(worker.ID, 10)
			if wid == "0" || wid == "" {
				if parts := strings.Split(worker.Token, ":"); len(parts) > 0 {
					wid = parts[0]
				}
			}
			if fid, ok := track.Telegram.FileIDs[wid]; ok && fid != "" {
				chosenWorker = worker
				fileID = fid
				atomic.AddInt64(&worker.Workload, 1)
				break
			}
		}
	}

	// 2. If not matched, pick the least-loaded worker
	if chosenWorker == nil {
		chosenWorker = s.AcquireWorker()
	}
	defer s.ReleaseWorker(chosenWorker)

	wid := strconv.FormatInt(chosenWorker.ID, 10)
	if wid == "0" || wid == "" {
		if parts := strings.Split(chosenWorker.Token, ":"); len(parts) > 0 {
			wid = parts[0]
		}
	}

	if fileID == "" && track.Telegram.FileIDs != nil {
		fileID = track.Telegram.FileIDs[wid]
	}

	fetchFresh := func() (string, error) {
		if track.CacheChatID != 0 && track.CacheMessageID != 0 {
			f, err := chosenWorker.FetchFileIDForMessage(ctx, track.CacheChatID, track.CacheMessageID)
			if err == nil && f != "" {
				return f, nil
			}
		}
		if track.SourceChatID != 0 && track.SourceMessageID != 0 {
			f, err := chosenWorker.FetchFileIDForMessage(ctx, track.SourceChatID, track.SourceMessageID)
			if err == nil && f != "" {
				return f, nil
			}
		}
		return "", errors.New("could not resolve message to fetch fresh file_id")
	}

	if fileID == "" {
		if fresh, err := fetchFresh(); err == nil && fresh != "" {
			fileID = fresh
		} else {
			fileID = track.Telegram.FileID
		}
	}

	if fileID == "" {
		return errors.New("no file_id available for track")
	}

	doDownloadThumb := func(fid string) error {
		decoded, err := DecodeFileID(fid)
		if err != nil {
			return fmt.Errorf("failed to decode file_id: %w", err)
		}

		thumbSizes := []string{"m", "s", "x", "y"}
		var lastErr error

		for _, size := range thumbSizes {
			loc := &tg.InputDocumentFileLocation{
				ID:            decoded.MediaID,
				AccessHash:    decoded.AccessHash,
				FileReference: decoded.FileReference,
				ThumbSize:     size,
			}

			dl := chosenWorker.Client.Downloader().WithPartSize(64 * 1024)
			var buf bytes.Buffer
			// Cap thumbnail download to 256KB to avoid streaming entire audio files
			cappedWriter := &partialWriter{
				w:      &buf,
				remain: 256 * 1024,
			}
			_, dlErr := dl.Download(chosenWorker.API, loc).Stream(ctx, cappedWriter)
			if dlErr == nil || errors.Is(dlErr, io.EOF) {
				data := buf.Bytes()
				if len(data) > 0 && isImageBytes(data) {
					_, writeErr := io.Copy(w, &buf)
					return writeErr
				}
			}
			lastErr = dlErr
		}
		if lastErr == nil {
			lastErr = errors.New("no document thumbnail found")
		}
		return lastErr
	}

	err := doDownloadThumb(fileID)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "FILE_REFERENCE") || strings.Contains(errStr, "400") {
			if fresh, refreshErr := fetchFresh(); refreshErr == nil && fresh != "" {
				fileID = fresh
				err = doDownloadThumb(fileID)
			}
		}
	}

	return err
}
