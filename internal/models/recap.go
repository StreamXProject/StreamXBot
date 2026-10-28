package models

// RecapPeriodType represents the time period of a recap.
type RecapPeriodType string

const (
	RecapPeriodWeekly  RecapPeriodType = "weekly"
	RecapPeriodMonthly RecapPeriodType = "monthly"
	RecapPeriodYearly  RecapPeriodType = "yearly"
)

// TrackStat represents a track's listening statistics in a recap.
type TrackStat struct {
	Track   *BrowseItem `json:"track"`
	Plays   int         `json:"plays"`
	Minutes float64     `json:"minutes"`
}

// ArtistStat represents an artist's listening statistics in a recap.
type ArtistStat struct {
	Name     string  `json:"name"`
	Plays    int     `json:"plays"`
	Minutes  float64 `json:"minutes"`
	CoverURL *string `json:"cover_url,omitempty"`
}

// AlbumStat represents an album's listening statistics in a recap.
type AlbumStat struct {
	Album    string  `json:"album"`
	Artist   string  `json:"artist"`
	AlbumID  *string `json:"album_id,omitempty"`
	CoverURL *string `json:"cover_url,omitempty"`
	Plays    int     `json:"plays"`
	Minutes  float64 `json:"minutes"`
}

// GenreStat represents a genre or topic's listening statistics in a recap.
type GenreStat struct {
	Name  string `json:"name"`
	Plays int    `json:"plays"`
}

// DayStat represents listening minutes on a specific date.
type DayStat struct {
	Date    string `json:"date"`
	Minutes int    `json:"minutes"`
}

// MonthStat represents listening minutes in a specific month.
type MonthStat struct {
	Month   string `json:"month"`
	Minutes int    `json:"minutes"`
}

// RecapStats represents the aggregated listening statistics for a recap snapshot.
type RecapStats struct {
	TotalMinutes          int           `json:"totalMinutes"`
	TotalPlays            int           `json:"totalPlays"`
	CompletedPlays        int           `json:"completedPlays"`
	SkippedPlays          int           `json:"skippedPlays"`
	UniqueTracks          int           `json:"uniqueTracks"`
	UniqueArtists         int           `json:"uniqueArtists"`
	UniqueAlbums          int           `json:"uniqueAlbums"`
	TopTracks             []*TrackStat  `json:"topTracks"`
	TopArtists            []*ArtistStat `json:"topArtists"`
	TopAlbums             []*AlbumStat  `json:"topAlbums"`
	TopGenres             []*GenreStat  `json:"topGenres"`
	ListeningByHour       []int         `json:"listeningByHour"`
	ListeningByWeekday    []int         `json:"listeningByWeekday"`
	ListeningByDay        []*DayStat    `json:"listeningByDay"`
	ListeningByMonth      []*MonthStat  `json:"listeningByMonth"`
	DiscoveryCount        int           `json:"discoveryCount"`
	DiscoveryTracks       []*BrowseItem `json:"discoveryTracks"`
	RepeatCount           int           `json:"repeatCount"`
	MostActiveHour        *int          `json:"mostActiveHour"`
	MostActiveWeekday     *int          `json:"mostActiveWeekday"`
	MostActiveDate        *string       `json:"mostActiveDate"`
	MostActiveDateMinutes int           `json:"mostActiveDateMinutes"`
	LongestSessionMinutes int           `json:"longestSessionMinutes"`
	SessionCount          int           `json:"sessionCount"`
	NightShare            float64       `json:"nightShare"`
	EarlyShare            float64       `json:"earlyShare"`
	WeekendShare          float64       `json:"weekendShare"`
	LosslessShare         float64       `json:"losslessShare"`
	TopArtistShare        float64       `json:"topArtistShare"`
}

// RecapPersonality represents a deterministic listening personality classification.
type RecapPersonality struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// RecapComparison represents comparison statistics against the preceding period.
type RecapComparison struct {
	Period          string  `json:"period"`
	TotalMinutes    int     `json:"totalMinutes"`
	TotalPlays      int     `json:"totalPlays"`
	UniqueArtists   int     `json:"uniqueArtists"`
	UniqueTracks    int     `json:"uniqueTracks"`
	MinutesDeltaPct *int    `json:"minutesDeltaPct"`
	PlaysDeltaPct   *int    `json:"playsDeltaPct"`
	ArtistsDelta    int     `json:"artistsDelta"`
	NightShareDelta int     `json:"nightShareDelta"`
	TopArtistPrev   *string `json:"topArtistPrev"`
}

// RecapSnapshot is the standardized snapshot schema version 1.
type RecapSnapshot struct {
	SchemaVersion int                 `json:"schemaVersion" bson:"schemaVersion"`
	Type          RecapPeriodType     `json:"type" bson:"type"`
	Period        string              `json:"period" bson:"period"`
	Label         string              `json:"label" bson:"label"`
	Start         float64             `json:"start" bson:"start"`
	End           float64             `json:"end" bson:"end"`
	Ongoing       bool                `json:"ongoing" bson:"ongoing"`
	GeneratedAt   float64             `json:"generatedAt" bson:"generatedAt"`
	TzOffsetMin   int                 `json:"tzOffsetMin,omitempty" bson:"tzOffsetMin,omitempty"`
	EventCount    int                 `json:"eventCount" bson:"eventCount"`
	LegacyData    bool                `json:"legacyData" bson:"legacyData"`
	Stats         *RecapStats         `json:"stats" bson:"stats"`
	Personality   []*RecapPersonality `json:"personality" bson:"personality"`
	Comparison    *RecapComparison    `json:"comparison" bson:"comparison"`
}

// RecapAvailableItem describes an unlocked recap period available for a user.
type RecapAvailableItem struct {
	Type    RecapPeriodType `json:"type"`
	Period  string          `json:"period"`
	Label   string          `json:"label"`
	Ongoing bool            `json:"ongoing"`
}

// RecapPublicTrack is a simplified track item for public share links.
type RecapPublicTrack struct {
	Title    *string `json:"title"`
	Artist   *string `json:"artist"`
	CoverURL *string `json:"cover_url"`
}

// RecapPublicSummary is the sanitized public representation of a shared recap.
type RecapPublicSummary struct {
	Type          RecapPeriodType     `json:"type"`
	Period        string              `json:"period"`
	Label         string              `json:"label"`
	TotalMinutes  int                 `json:"totalMinutes"`
	TotalPlays    int                 `json:"totalPlays"`
	UniqueArtists int                 `json:"uniqueArtists"`
	UniqueTracks  int                 `json:"uniqueTracks"`
	TopArtist     *ArtistStat         `json:"topArtist"`
	TopTrack      *RecapPublicTrack   `json:"topTrack"`
	Personality   []*RecapPersonality `json:"personality"`
}

// RecapShareItem represents an item in GET /me/recaps/shares.
type RecapShareItem struct {
	Token     string          `json:"token"`
	Type      RecapPeriodType `json:"type"`
	Period    string          `json:"period"`
	Label     string          `json:"label,omitempty"`
	CreatedAt float64         `json:"created_at"`
}

// ListeningEventDoc represents a single row stored in the listening_events collection.
type ListeningEventDoc struct {
	ID         string  `bson:"_id"`
	UserID     int64   `bson:"user_id"`
	TrackID    string  `bson:"track_id"`
	PlayedAt   float64 `bson:"played_at"`
	StartedAt  float64 `bson:"started_at"`
	PlayedMs   *int    `bson:"played_ms"`
	DurationMs int     `bson:"duration_ms"`
	Completed  bool    `bson:"completed"`
	Skipped    bool    `bson:"skipped"`
	Source     string  `bson:"source"`
	SessionID  *string `bson:"session_id,omitempty"`
	CreatedAt  float64 `bson:"created_at"`
	Legacy     bool    `bson:"legacy,omitempty"`
}

// RecapShareDoc represents a share link document in the recap_shares collection.
type RecapShareDoc struct {
	Token     string              `bson:"token"`
	UserID    int64               `bson:"user_id"`
	Type      RecapPeriodType     `bson:"type"`
	Period    string              `bson:"period"`
	CreatedAt float64             `bson:"created_at"`
	RevokedAt *float64            `bson:"revoked_at,omitempty"`
	Summary   *RecapPublicSummary `bson:"summary"`
}
