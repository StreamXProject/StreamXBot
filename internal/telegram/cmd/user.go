package cmd

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

// handlePing responds with latency and service uptime.
func (h *Handler) handlePing(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message) {
	start := time.Now()
	uptime := time.Since(h.startTime)
	latency := time.Since(start).Milliseconds()
	if latency <= 0 {
		latency = 1
	}
	uptimeStr := formatDuration(uptime)
	reply := fmt.Sprintf("<code>%d</code>ms \n| <code>%s</code>", latency, uptimeStr)
	_ = h.replyHTML(ctx, e, upd, reply, nil)
}

// handleID displays chat ID, sender user ID, or replied user ID.
func (h *Handler) handleID(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, chatID int64, senderID int64) {
	replyUser := int64(0)
	if msg.ReplyTo != nil {
		if header, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok && header.ReplyToMsgID != 0 {
			if w := h.tgService.PrimaryWorker(); w != nil && w.API != nil {
				res, err := w.API.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: header.ReplyToMsgID}})
				if err == nil {
					switch mList := res.(type) {
					case *tg.MessagesMessages:
						for _, m := range mList.Messages {
							if fullMsg, ok := m.(*tg.Message); ok && fullMsg.FromID != nil {
								if u, ok := fullMsg.FromID.(*tg.PeerUser); ok {
									replyUser = u.UserID
									break
								}
							}
						}
					case *tg.MessagesMessagesSlice:
						for _, m := range mList.Messages {
							if fullMsg, ok := m.(*tg.Message); ok && fullMsg.FromID != nil {
								if u, ok := fullMsg.FromID.(*tg.PeerUser); ok {
									replyUser = u.UserID
									break
								}
							}
						}
					case *tg.MessagesChannelMessages:
						for _, m := range mList.Messages {
							if fullMsg, ok := m.(*tg.Message); ok && fullMsg.FromID != nil {
								if u, ok := fullMsg.FromID.(*tg.PeerUser); ok {
									replyUser = u.UserID
									break
								}
							}
						}
					}
				}
			}
		}
	}

	if replyUser != 0 {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Replied user ID: <code>%d</code>", replyUser), nil)
		return
	}

	text := fmt.Sprintf("Chat ID: <code>%d</code>", chatID)
	if senderID != 0 {
		text += fmt.Sprintf("\nYour User ID: <code>%d</code>", senderID)
	}

	_ = h.replyHTML(ctx, e, upd, text, nil)
}

// handleSearch queries tracks by title, artist, or album.
func (h *Handler) handleSearch(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, args []string) {
	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "Usage:\n/search &lt;query&gt;", nil)
		return
	}

	query := strings.Join(args, " ")
	if h.trackSvc == nil {
		_ = h.replyHTML(ctx, e, upd, "Track search service unavailable.", nil)
		return
	}

	results, err := h.trackSvc.Search(ctx, query, 5)
	if err != nil || len(results) == 0 {
		_ = h.replyHTML(ctx, e, upd, "No results found.", nil)
		return
	}

	var sb strings.Builder
	for i, item := range results {
		trackName := html.EscapeString(item.Title)
		artist := html.EscapeString(item.Artist)
		album := html.EscapeString(item.Album)

		sb.WriteString(fmt.Sprintf("🎵 <code>%s</code>\n", trackName))
		if artist != "" {
			sb.WriteString(fmt.Sprintf("👤 %s\n", artist))
		}
		if album != "" {
			sb.WriteString(fmt.Sprintf("💿 %s\n", album))
		}
		durStr := formatSeconds(item.DurationSec)
		sb.WriteString(fmt.Sprintf("%s · <code>%s</code>\n", durStr, item.ID))

		if i < len(results)-1 {
			sb.WriteString("\n")
		}
	}

	_ = h.replyHTML(ctx, e, upd, sb.String(), nil)
}

// handleMediaInfo inspects technical metadata of an attached or replied audio file.
func (h *Handler) handleMediaInfo(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message) {
	var targetDoc *tg.Document

	if msg.Media != nil {
		if docMedia, ok := msg.Media.(*tg.MessageMediaDocument); ok && docMedia.Document != nil {
			if doc, ok := docMedia.Document.(*tg.Document); ok {
				targetDoc = doc
			}
		}
	}

	if targetDoc == nil {
		_ = h.replyHTML(ctx, e, upd, "Usage:\n/mediainfo &lt;link&gt;\nor reply to an audio / media / link", nil)
		return
	}

	var title, artist, fileName string
	var duration int
	for _, attr := range targetDoc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeAudio:
			title = a.Title
			artist = a.Performer
			duration = a.Duration
		case *tg.DocumentAttributeFilename:
			fileName = a.FileName
		}
	}

	sizeMB := float64(targetDoc.Size) / (1024 * 1024)
	var sb strings.Builder
	sb.WriteString("<b>Media Information:</b>\n\n")
	if title != "" {
		sb.WriteString(fmt.Sprintf("• <b>Title:</b> %s\n", html.EscapeString(title)))
	}
	if artist != "" {
		sb.WriteString(fmt.Sprintf("• <b>Artist:</b> %s\n", html.EscapeString(artist)))
	}
	if fileName != "" {
		sb.WriteString(fmt.Sprintf("• <b>File:</b> <code>%s</code>\n", html.EscapeString(fileName)))
	}
	sb.WriteString(fmt.Sprintf("• <b>MIME:</b> <code>%s</code>\n", html.EscapeString(targetDoc.MimeType)))
	sb.WriteString(fmt.Sprintf("• <b>Size:</b> %.2f MB\n", sizeMB))
	if duration > 0 {
		sb.WriteString(fmt.Sprintf("• <b>Duration:</b> %s\n", formatSeconds(int32(duration))))
	}

	_ = h.replyHTML(ctx, e, upd, sb.String(), nil)
}
