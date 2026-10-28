package models

// UserHistoryRecord represents a playback record in MongoDB userHistory.
type UserHistoryRecord struct {
	ID       string  `bson:"_id,omitempty" json:"id,omitempty"`
	UserID   int64   `bson:"user_id" json:"user_id"`
	TrackID  string  `bson:"track_id" json:"track_id"`
	PlayedAt float64 `bson:"played_at" json:"played_at"`
}

// UserPlaybackRecord represents an aggregated record in userPlayback.
type UserPlaybackRecord struct {
	ID       string  `bson:"_id" json:"id"` // user_id:track_id:bucket
	UserID   int64   `bson:"user_id" json:"user_id"`
	TrackID  string  `bson:"track_id" json:"track_id"`
	PlayedAt float64 `bson:"played_at" json:"played_at"`
	Bucket   int64   `bson:"bucket" json:"bucket"`
	Source   string  `bson:"source,omitempty" json:"source,omitempty"`
	Plays    int64   `bson:"plays,omitempty" json:"plays,omitempty"`
}
