package models

// UserPlaylist represents a custom playlist in the user_playlists collection.
type UserPlaylist struct {
	ID              string  `bson:"_id" json:"playlist_id"`
	UserID          int64   `bson:"user_id" json:"user_id"`
	Name            string  `bson:"name" json:"name"`
	CoverID         string  `bson:"cover_id,omitempty" json:"cover_id,omitempty"`
	CoverURL        string  `bson:"cover_url,omitempty" json:"cover_url,omitempty"`
	NormalThumbnail string  `bson:"normal_thumbnail,omitempty" json:"normal_thumbnail,omitempty"`
	CollageHash     string  `bson:"collage_hash,omitempty" json:"collage_hash,omitempty"`
	CreatedAt       float64 `bson:"created_at" json:"created_at"`
	UpdatedAt       float64 `bson:"updated_at" json:"updated_at"`
}

// PlaylistTrack represents a track entry in the playlist_tracks collection.
type PlaylistTrack struct {
	ID         string  `bson:"_id,omitempty" json:"_id,omitempty"`
	PlaylistID string  `bson:"playlist_id" json:"playlist_id"`
	TrackID    string  `bson:"track_id" json:"track_id"`
	Position   int     `bson:"position" json:"position"`
	AddedAt    float64 `bson:"added_at" json:"added_at"`
}

// PlaylistItem represents playlist data returned in lists.
type PlaylistItem struct {
	PlaylistID      string   `json:"playlist_id"`
	Name            string   `json:"name"`
	Thumbnails      []string `json:"thumbnails"`
	CoverURL        string   `json:"cover_url,omitempty"`
	NormalThumbnail string   `json:"normal_thumbnail,omitempty"`
	CoverID         string   `json:"cover_id,omitempty"`
	CreatedAt       float64  `json:"created_at,omitempty"`
	UpdatedAt       float64  `json:"updated_at,omitempty"`
}

// PlaylistCreate request payload for creating a new playlist.
type PlaylistCreate struct {
	Name string `json:"name"`
}

// PlaylistRename request payload for renaming a playlist.
type PlaylistRename struct {
	Name string `json:"name"`
}

// PlaylistTrackAdd request payload for adding tracks to a playlist.
type PlaylistTrackAdd struct {
	TrackID  any      `json:"track_id,omitempty"`
	TrackIDs []string `json:"track_ids,omitempty"`
}

// PlaylistsResponse is the list of user playlists.
type PlaylistsResponse struct {
	Items []PlaylistItem `json:"items"`
}

// PlaylistTracksResponse contains paginated tracks for a playlist.
type PlaylistTracksResponse struct {
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Total   int64         `json:"total"`
	Items   []*BrowseItem `json:"items"`
}

// PlaylistShareResponse returns public read-only shared playlist details.
type PlaylistShareResponse struct {
	PlaylistID      string        `json:"playlist_id"`
	Name            string        `json:"name"`
	CoverURL        string        `json:"cover_url,omitempty"`
	NormalThumbnail string        `json:"normal_thumbnail,omitempty"`
	Tracks          []*BrowseItem `json:"tracks"`
	CreatedAt       float64       `json:"created_at,omitempty"`
}

// UserAlbum represents a saved album in user_albums collection.
type UserAlbum struct {
	ID        string  `bson:"_id,omitempty" json:"_id,omitempty"`
	UserID    int64   `bson:"user_id" json:"user_id"`
	AlbumID   string  `bson:"album_id" json:"album_id"`
	SavedAt   float64 `bson:"saved_at" json:"saved_at"`
	UpdatedAt float64 `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// UserAlbumAdd payload for saving an album.
type UserAlbumAdd struct {
	AlbumID  any      `json:"album_id,omitempty"`
	AlbumIDs []string `json:"album_ids,omitempty"`
}

// UserAlbumItem represents a single album entry in list saved albums.
type UserAlbumItem struct {
	AlbumID string  `json:"album_id"`
	Album   any     `json:"album,omitempty"`
	SavedAt float64 `json:"saved_at,omitempty"`
}

// UserAlbumsResponse list of saved albums.
type UserAlbumsResponse struct {
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
	Total   int64           `json:"total"`
	Items   []UserAlbumItem `json:"items"`
}

// UserFavourite represents a track liked by a user in user_favourites.
type UserFavourite struct {
	ID        string  `bson:"_id,omitempty" json:"_id,omitempty"`
	UserID    int64   `bson:"user_id" json:"user_id"`
	TrackID   string  `bson:"track_id" json:"track_id"`
	CreatedAt float64 `bson:"created_at" json:"created_at"`
	UpdatedAt float64 `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// UserFavouriteArtist represents a followed artist in user_favourite_artists.
type UserFavouriteArtist struct {
	ID        string  `bson:"_id,omitempty" json:"_id,omitempty"`
	UserID    int64   `bson:"user_id" json:"user_id"`
	ArtistID  string  `bson:"artist_id" json:"artist_id"`
	CreatedAt float64 `bson:"created_at" json:"created_at"`
	UpdatedAt float64 `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// FavouriteCreate payload for liking a track.
type FavouriteCreate struct {
	TrackID string `json:"track_id"`
}

// ArtistFavouriteCreate payload for liking an artist.
type ArtistFavouriteCreate struct {
	ArtistID string `json:"artist_id,omitempty"`
	ID       string `json:"id,omitempty"`
	ArtistId string `json:"artistId,omitempty"`
}

// FavouriteItem pairs a track with the favourited timestamp.
type FavouriteItem struct {
	Track     *BrowseItem `json:"track"`
	CreatedAt float64     `json:"created_at,omitempty"`
}

// FavouritesResponse contains paginated favourited track items.
type FavouritesResponse struct {
	Page          int             `json:"page"`
	PerPage       int             `json:"per_page"`
	Total         int64           `json:"total"`
	Items         []FavouriteItem `json:"items"`
	LastUpdatedAt *float64        `json:"last_updated_at,omitempty"`
}

// FavouriteIdsResponse contains array of favourited track IDs.
type FavouriteIdsResponse struct {
	Page          int      `json:"page"`
	PerPage       int      `json:"per_page"`
	Total         int64    `json:"total"`
	IDs           []string `json:"ids"`
	Exists        bool     `json:"exists"`
	LastUpdatedAt *float64 `json:"last_updated_at,omitempty"`
}

// FavouriteIDsResponse alias for FavouriteIdsResponse.
type FavouriteIDsResponse = FavouriteIdsResponse

// FavouriteArtistsResponse contains array of followed artist IDs.
type FavouriteArtistsResponse struct {
	OK  bool     `json:"ok"`
	IDs []string `json:"ids"`
}

// ListeningEventItem represents a single telemetry playback event in listening_events.
type ListeningEventItem struct {
	ID         string  `json:"id" bson:"event_id"`
	TrackID    string  `json:"track_id" bson:"track_id"`
	PlayedAt   float64 `json:"played_at" bson:"played_at"`
	StartedAt  float64 `json:"started_at" bson:"started_at"`
	PlayedMs   float64 `json:"played_ms" bson:"played_ms"`
	DurationMs float64 `json:"duration_ms" bson:"duration_ms"`
	Completed  bool    `json:"completed" bson:"completed"`
	Skipped    bool    `json:"skipped" bson:"skipped"`
	Source     string  `json:"source" bson:"source"`
	SessionID  string  `json:"session_id,omitempty" bson:"session_id,omitempty"`
}

// ListeningEventsPayload is the payload for batch listening events.
type ListeningEventsPayload struct {
	Events []ListeningEventItem `json:"events"`
}

// StartRemoteAuthResponse starts Discord Remote Auth session.
type StartRemoteAuthResponse struct {
	SessionID   string `json:"session_id"`
	Status      string `json:"status"`
	URL         string `json:"url,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// RemoteAuthStatusResponse returns status of Discord Remote Auth session.
type RemoteAuthStatusResponse struct {
	SessionID   string `json:"session_id"`
	Status      string `json:"status"`
	URL         string `json:"url,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	User        any    `json:"user,omitempty"`
	Token       string `json:"token,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ExternalAssetsRequest payload for proxying Discord asset requests.
type ExternalAssetsRequest struct {
	URLs          []string `json:"urls"`
	ApplicationID string   `json:"application_id,omitempty"`
	Token         string   `json:"token"`
}
