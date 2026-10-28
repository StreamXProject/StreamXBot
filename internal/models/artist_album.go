package models

// Artist represents an artist record in the artists collection.
type Artist struct {
	ID          string  `bson:"_id" json:"_id"`
	Id          string  `bson:"-" json:"id,omitempty"`
	Name        string  `bson:"name" json:"name"`
	MatchArtist string  `bson:"match_artist,omitempty" json:"-"`
	TracksCount int64   `bson:"tracks_count" json:"tracks_count"`
	TrackCount  int64   `bson:"-" json:"track_count,omitempty"`
	CoverURL         string  `bson:"cover_url,omitempty" json:"cover_url,omitempty"`
	AvatarURL        string  `bson:"-" json:"avatar_url,omitempty"`
	Followers        int64   `bson:"followers,omitempty" json:"followers,omitempty"`
	MonthlyListeners int64   `bson:"monthly_listeners,omitempty" json:"monthly_listeners,omitempty"`
	Bio              string  `bson:"bio,omitempty" json:"bio,omitempty"`
	IsFollowing      bool    `bson:"-" json:"is_following"`
	CreatedAt        float64 `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt        float64 `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// Album represents an album record in the albums collection.
type Album struct {
	ID            string   `bson:"_id" json:"_id"`
	Id            string   `bson:"-" json:"id,omitempty"`
	Title         string   `bson:"title" json:"title"`
	Artist        string   `bson:"artist,omitempty" json:"artist,omitempty"`
	Artists       []string `bson:"artists,omitempty" json:"artists,omitempty"`
	CoverURL      string   `bson:"cover_url,omitempty" json:"cover_url,omitempty"`
	DurationTotal float64  `bson:"duration_total,omitempty" json:"duration_total,omitempty"`
	MatchAlbum    string   `bson:"match_album,omitempty" json:"-"`
	MatchArtist   string   `bson:"match_artist,omitempty" json:"-"`
	TracksCount   int64    `bson:"tracks_count" json:"tracks_count"`
	TrackCount    int64    `bson:"-" json:"track_count,omitempty"`
	Year          int      `bson:"year,omitempty" json:"year,omitempty"`
	CreatedAt     float64  `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     float64  `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// ArtistDetail contains full artist information along with their top tracks and albums.
type ArtistDetail struct {
	Ok            bool          `json:"ok"`
	Artist        *Artist       `json:"artist"`
	PopularTracks []*BrowseItem `json:"popular_tracks"`
	TopTracks     []*BrowseItem `json:"top_tracks"`
	Releases      []*Album      `json:"releases"`
	Albums        []*Album      `json:"albums"`
	Singles       []*BrowseItem `json:"singles"`
	Tracks        []*BrowseItem `json:"tracks"`
}

// AlbumDetail contains full album metadata and its ordered tracklist.
type AlbumDetail struct {
	Ok     bool          `json:"ok"`
	Album  *Album        `json:"album"`
	Tracks []*BrowseItem `json:"tracks"`
}

// ArtistsResponse is a paginated list of artists matching Python schema.
type ArtistsResponse struct {
	Ok      bool      `json:"ok"`
	Page    int       `json:"page"`
	PerPage int       `json:"per_page"`
	Total   int64     `json:"total"`
	Items   []*Artist `json:"items"`
}

// AlbumsResponse is a paginated list of albums matching Python schema.
type AlbumsResponse struct {
	Ok      bool     `json:"ok"`
	Page    int      `json:"page"`
	PerPage int      `json:"per_page"`
	Total   int64    `json:"total"`
	Items   []*Album `json:"items"`
}
