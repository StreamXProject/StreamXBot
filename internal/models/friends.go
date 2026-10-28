package models

// FriendRequestPayload represents payload to send a friend request matching Api/schemas/friends.py.
type FriendRequestPayload struct {
	To int64 `json:"to"`
}

// AcceptRequestPayload represents payload to accept a friend request matching Api/schemas/friends.py.
type AcceptRequestPayload struct {
	UserID int64 `json:"userId"`
}

// InviteJamPayload represents payload to invite a friend to a jam session matching Api/schemas/friends.py.
type InviteJamPayload struct {
	ToUserID int64  `json:"toUserId"`
	JamID    string `json:"jamId"`
}

// SettingsPayload represents friend / jam privacy settings payload matching Api/schemas/friends.py.
type SettingsPayload struct {
	ShareListening  *string `json:"share_listening,omitempty"`
	AllowJamInvites *bool   `json:"allow_jam_invites,omitempty"`
}

// FcmTokenPayload represents FCM token update payload matching Api/schemas/friends.py.
type FcmTokenPayload struct {
	Token string `json:"token"`
}
