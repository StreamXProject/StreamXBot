package models

import "strings"

// AudioMeta represents metadata extracted from the audio stream or tags via MediaInfo.
type AudioMeta struct {
	Title          string   `bson:"title,omitempty" json:"title"`
	Album          string   `bson:"album,omitempty" json:"album,omitempty"`
	Artist         string   `bson:"artist,omitempty" json:"artist"`
	Composer       string   `bson:"composer,omitempty" json:"composer,omitempty"`
	Label          string   `bson:"label,omitempty" json:"label,omitempty"`
	Genre          string   `bson:"genre,omitempty" json:"genre,omitempty"`
	Year           *int32   `bson:"year,omitempty" json:"year,omitempty"`
	DurationSec    int32    `bson:"duration_sec" json:"duration_sec"`
	Type           string   `bson:"type,omitempty" json:"type,omitempty"`
	BitDepth       *int32   `bson:"bit_depth,omitempty" json:"bit_depth,omitempty"`
	BitrateKbps    *int32   `bson:"bitrate_kbps,omitempty" json:"bitrate_kbps,omitempty"`
	SamplingRateHz *int32   `bson:"sampling_rate_hz,omitempty" json:"sampling_rate_hz,omitempty"`
	Artists        []string `bson:"artists,omitempty" json:"artists,omitempty"`
	AlbumID        string   `bson:"album_id,omitempty" json:"album_id,omitempty"`

	// Legacy / Compatibility fields that should NOT be serialized to MongoDB audio subdocument
	Performer string         `bson:"performer,omitempty" json:"performer,omitempty"`
	CoverURL  string         `bson:"-" json:"cover_url,omitempty"`
	Lyrics    string         `bson:"-" json:"lyrics,omitempty"`
	Titles    map[string]any `bson:"titles,omitempty" json:"titles,omitempty"`
	MimeType  string         `bson:"-" json:"mime_type,omitempty"`
	FileSize  int64          `bson:"-" json:"file_size,omitempty"`
}

// TelegramMeta stores Telegram file references and chat mapping.
type TelegramMeta struct {
	FileID       string            `bson:"file_id,omitempty" json:"file_id"`
	FileUniqueID string            `bson:"file_unique_id,omitempty" json:"file_unique_id,omitempty"`
	FileName     string            `bson:"file_name,omitempty" json:"file_name,omitempty"`
	FileSize     int64             `bson:"file_size,omitempty" json:"file_size,omitempty"`
	MimeType     string            `bson:"mime_type,omitempty" json:"mime_type,omitempty"`
	FileIDs      map[string]string `bson:"file_ids,omitempty" json:"file_ids,omitempty"`
}

// SpotifyMeta stores resolved Spotify match data and cover URLs.
type SpotifyMeta struct {
	URL                   string `bson:"url,omitempty" json:"url,omitempty"`
	CoverURL              string `bson:"cover_url,omitempty" json:"cover_url,omitempty"`
	BigCoverURL           string `bson:"big_cover_url,omitempty" json:"big_cover_url,omitempty"`
	CoverSource           string `bson:"cover_source,omitempty" json:"cover_source,omitempty"`
	ArtistAvatar          string `bson:"artist_avatar,omitempty" json:"artist_avatar,omitempty"`
	TrackSpotifyID        string `bson:"track_spotify_id,omitempty" json:"track_spotify_id,omitempty"`
	CloudflareCoverURL    string `bson:"cloudflare_cover_url,omitempty" json:"cloudflare_cover_url,omitempty"`
	CloudflareBigCoverURL string `bson:"cloudflare_big_cover_url,omitempty" json:"cloudflare_big_cover_url,omitempty"`
}

// Track represents the full track document stored in the audioTracks MongoDB collection.
type Track struct {
	ID                    string         `bson:"_id" json:"_id"`
	Audio                 AudioMeta      `bson:"audio" json:"audio"`
	Telegram              TelegramMeta   `bson:"telegram" json:"telegram"`
	Spotify               SpotifyMeta    `bson:"spotify,omitempty" json:"spotify,omitempty"`
	CoverURL              string         `bson:"-" json:"cover_url,omitempty"`
	CloudflareCoverURL    string         `bson:"cloudflare_cover_url,omitempty" json:"cloudflare_cover_url,omitempty"`
	CloudflareBigCoverURL string         `bson:"cloudflare_big_cover_url,omitempty" json:"cloudflare_big_cover_url,omitempty"`
	Lyrics                string         `bson:"lyrics,omitempty" json:"lyrics,omitempty"`
	LyricsCache           map[string]any `bson:"lyrics_cache,omitempty" json:"lyrics_cache,omitempty"`
	Titles                map[string]any `bson:"titles,omitempty" json:"titles,omitempty"`
	Fingerprint           string         `bson:"fingerprint,omitempty" json:"fingerprint,omitempty"`
	ContentHash           string         `bson:"content_hash,omitempty" json:"content_hash,omitempty"`
	SourceChatID          int64          `bson:"source_chat_id,omitempty" json:"source_chat_id,omitempty"`
	SourceMessageID       int32          `bson:"source_message_id,omitempty" json:"source_message_id,omitempty"`
	CacheChatID           int64          `bson:"cache_chat_id,omitempty" json:"cache_chat_id,omitempty"`
	CacheMessageID        int32          `bson:"cache_message_id,omitempty" json:"cache_message_id,omitempty"`
	TopicID               int32          `bson:"topic_id,omitempty" json:"topic_id"`
	TopicName             string         `bson:"topic_name,omitempty" json:"topic_name,omitempty"`
	PlayCount             int32          `bson:"play_count,omitempty" json:"play_count"`
	LikesCount            int64          `bson:"likes_count,omitempty" json:"likes_count"`
	CreatedAt             float64        `bson:"created_at,omitempty" json:"created_at"`
	UpdatedAt             float64        `bson:"updated_at,omitempty" json:"updated_at"`
	EnrichedAt            float64        `bson:"enriched_at,omitempty" json:"enriched_at"`
	Enriched              bool           `bson:"enriched" json:"enriched"`
	Indexed               bool           `bson:"indexed" json:"indexed"`
	Deleted               bool           `bson:"deleted,omitempty" json:"deleted"`
	Liked                 bool           `bson:"-" json:"liked"`
}

// EffectiveCoverURL returns the best available preview/list cover art URL, prioritizing Cloudflare R2 preview thumbnails.
func (t *Track) EffectiveCoverURL() string {
	if t.CloudflareCoverURL != "" {
		return t.CloudflareCoverURL
	}
	if t.Spotify.CloudflareCoverURL != "" {
		return t.Spotify.CloudflareCoverURL
	}
	if t.Spotify.CoverURL != "" {
		return t.Spotify.CoverURL
	}
	if t.CloudflareBigCoverURL != "" {
		return t.CloudflareBigCoverURL
	}
	if t.Spotify.CloudflareBigCoverURL != "" {
		return t.Spotify.CloudflareBigCoverURL
	}
	if t.Spotify.BigCoverURL != "" {
		return t.Spotify.BigCoverURL
	}
	return t.Audio.CoverURL
}

// EffectiveBigCoverURL returns the best available high-resolution cover art URL, prioritizing Cloudflare R2 master artwork.
func (t *Track) EffectiveBigCoverURL() string {
	if t.CloudflareBigCoverURL != "" {
		return t.CloudflareBigCoverURL
	}
	if t.Spotify.CloudflareBigCoverURL != "" {
		return t.Spotify.CloudflareBigCoverURL
	}
	if t.Spotify.BigCoverURL != "" {
		return t.Spotify.BigCoverURL
	}
	return t.EffectiveCoverURL()
}

// EffectiveArtist returns the best artist/performer string.
func (t *Track) EffectiveArtist() string {
	if t.Audio.Artist != "" {
		return t.Audio.Artist
	}
	return t.Audio.Performer
}

// EffectiveTitles returns the best available alternative titles map (original, romanized, etc.).
func (t *Track) EffectiveTitles() map[string]any {
	if len(t.Titles) > 0 {
		return t.Titles
	}
	if len(t.Audio.Titles) > 0 {
		return t.Audio.Titles
	}
	return nil
}

// EffectiveType returns the audio format type, automatically distinguishing ALAC from lossy M4A if bit depth is present.
func (t *Track) EffectiveType() string {
	typ := strings.ToLower(strings.TrimSpace(t.Audio.Type))
	if (typ == "m4a" || typ == "" || typ == "aac") && t.Audio.BitDepth != nil && *t.Audio.BitDepth > 0 {
		return "alac"
	}
	if typ != "" {
		return typ
	}
	name := strings.ToLower(t.Telegram.FileName)
	if strings.Contains(name, "alac") || strings.Contains(strings.ToLower(t.Telegram.MimeType), "alac") {
		return "alac"
	}
	return typ
}

// BrowseItem represents a flattened track item optimized for list/feed UI views.
type BrowseItem struct {
	ID                    string         `json:"id"`
	DocID                 string         `json:"_id,omitempty"`
	SourceChatID          int64          `json:"source_chat_id,omitempty"`
	SourceMessageID       int32          `json:"source_message_id,omitempty"`
	TopicID               int32          `json:"topic_id,omitempty"`
	TopicName             string         `json:"topic_name,omitempty"`
	Title                 string         `json:"title"`
	Artist                string         `json:"artist"`
	Album                 string         `json:"album,omitempty"`
	AlbumID               string         `json:"album_id,omitempty"`
	DurationSec           int32          `json:"duration_sec"`
	Type                  string         `json:"type,omitempty"`
	SamplingRateHz        *int32         `json:"sampling_rate_hz,omitempty"`
	BitDepth              *int32         `json:"bit_depth,omitempty"`
	BitrateKbps           *int32         `json:"bitrate_kbps,omitempty"`
	SpotifyURL            string         `json:"spotify_url,omitempty"`
	CoverURL              string         `json:"cover_url,omitempty"`
	BigCoverURL           string         `json:"big_cover_url,omitempty"`
	CloudflareCoverURL    string         `json:"cloudflare_cover_url,omitempty"`
	CloudflareBigCoverURL string         `json:"cloudflare_big_cover_url,omitempty"`
	Titles                map[string]any `json:"titles,omitempty"`
	CreatedAt             float64        `json:"created_at"`
	UpdatedAt             float64        `json:"updated_at"`
	Liked                 bool           `json:"liked"`
}

// ToBrowseItem converts a full Track entity into a lightweight BrowseItem.
func (t *Track) ToBrowseItem() *BrowseItem {
	cfCover := t.CloudflareCoverURL
	if cfCover == "" {
		cfCover = t.Spotify.CloudflareCoverURL
	}
	cfBigCover := t.CloudflareBigCoverURL
	if cfBigCover == "" {
		cfBigCover = t.Spotify.CloudflareBigCoverURL
	}

	return &BrowseItem{
		ID:                    t.ID,
		DocID:                 t.ID,
		SourceChatID:          t.SourceChatID,
		SourceMessageID:       t.SourceMessageID,
		TopicID:               t.TopicID,
		TopicName:             t.TopicName,
		Title:                 t.Audio.Title,
		Artist:                t.EffectiveArtist(),
		Album:                 t.Audio.Album,
		AlbumID:               t.Audio.AlbumID,
		DurationSec:           t.Audio.DurationSec,
		Type:                  t.EffectiveType(),
		SamplingRateHz:        t.Audio.SamplingRateHz,
		BitDepth:              t.Audio.BitDepth,
		BitrateKbps:           t.Audio.BitrateKbps,
		SpotifyURL:            t.Spotify.URL,
		CoverURL:              t.EffectiveCoverURL(),
		BigCoverURL:           t.EffectiveBigCoverURL(),
		CloudflareCoverURL:    cfCover,
		CloudflareBigCoverURL: cfBigCover,
		Titles:                t.EffectiveTitles(),
		CreatedAt:             t.CreatedAt,
		UpdatedAt:             t.UpdatedAt,
		Liked:                 t.Liked,
	}
}

// BrowseResponse represents a paginated response of BrowseItem objects matching Api/schemas/browse.py.
type BrowseResponse struct {
	Items      []*BrowseItem `json:"items"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PerPage    int           `json:"per_page"`
	TotalPages int           `json:"total_pages,omitempty"`
	CoverURL   string        `json:"cover_url,omitempty"`
}

// TopicItem represents an aggregated topic with track counts and cover image matching Api/schemas/topics.py.
type TopicItem struct {
	Name            string   `json:"name"`
	TopicName       string   `json:"topic_name"`
	TopicID         *int64   `json:"topic_id,omitempty"`
	Count           int64    `json:"count"`
	TracksCount     int64    `json:"tracks_count"`
	CoverURL        string   `json:"cover_url,omitempty"`
	ThumbnailURL    string   `json:"thumbnail_url,omitempty"`
	NormalThumbnail string   `json:"normal_thumbnail,omitempty"`
	Thumbnails      []string `json:"thumbnails"`
	SourceChatID    *int64   `json:"source_chat_id,omitempty"`
	Endpoint        string   `json:"endpoint"`
}

// TopicsResponse is the response payload for GET /topics matching Api/schemas/topics.py.
type TopicsResponse struct {
	OK     bool         `json:"ok"`
	Total  int          `json:"total"`
	Items  []*TopicItem `json:"items"`
	Topics []string     `json:"topics"`
}

// TrackResponse represents a full track document matching Api/schemas/track.py.
type TrackResponse struct {
	ID              string         `json:"_id"`
	DocID           string         `json:"id,omitempty"`
	SourceChatID    *int64         `json:"source_chat_id,omitempty"`
	SourceMessageID *int32         `json:"source_message_id,omitempty"`
	Telegram        map[string]any `json:"telegram,omitempty"`
	Audio           map[string]any `json:"audio,omitempty"`
	Spotify         map[string]any `json:"spotify,omitempty"`
	Titles          map[string]any `json:"titles,omitempty"`
	ContentHash     string         `json:"content_hash,omitempty"`
	Fingerprint     string         `json:"fingerprint,omitempty"`
	CreatedAt       float64        `json:"created_at,omitempty"`
	UpdatedAt       float64        `json:"updated_at,omitempty"`
	Liked           bool           `json:"liked"`
}

// ChannelItem represents an indexed Telegram source channel.
type ChannelItem struct {
	ID       int64  `json:"id"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
	Type     string `json:"type,omitempty"`
}

// ChannelIDsResponse is the response payload for GET /channelids.
type ChannelIDsResponse struct {
	OK    bool          `json:"ok"`
	Items []ChannelItem `json:"items"`
}
