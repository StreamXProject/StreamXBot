package models

// AvailablePlaylistItem metadata for an available daily playlist.
type AvailablePlaylistItem struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Name            string   `json:"name"`
	Thumbnails      []string `json:"thumbnails"`
	ThumbnailURL    string   `json:"thumbnail_url"`
	NormalThumbnail string   `json:"normal_thumbnail"`
	Endpoint        string   `json:"endpoint"`
	RequiresAuth    bool     `json:"requires_auth"`
}

// AvailablePlaylistsResponse response for GET /playlists/available.
type AvailablePlaylistsResponse struct {
	Items []*AvailablePlaylistItem `json:"items"`
}

// DailyPlaylistDoc represents a generated daily playlist in MongoDB.
type DailyPlaylistDoc struct {
	ID              string   `bson:"_id" json:"id"` // key:date:channel_id
	Key             string   `bson:"key" json:"key"`
	Date            string   `bson:"date" json:"date"`
	ChannelID       int64    `bson:"channel_id,omitempty" json:"channel_id,omitempty"`
	TrackIDs        []string `bson:"track_ids" json:"track_ids"`
	ThumbnailURL    string   `bson:"thumbnail_url,omitempty" json:"thumbnail_url,omitempty"`
	NormalThumbnail string   `bson:"normal_thumbnail,omitempty" json:"normal_thumbnail,omitempty"`
	GeneratedAt     float64  `bson:"generated_at" json:"generated_at"`
}
