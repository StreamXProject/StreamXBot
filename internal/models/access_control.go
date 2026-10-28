package models

// RequiredChat represents a Telegram channel or group users must join.
type RequiredChat struct {
	ChatID     int64  `bson:"chat_id" json:"chat_id"`
	Title      string `bson:"title,omitempty" json:"title,omitempty"`
	InviteLink string `bson:"invite_link,omitempty" json:"invite_link,omitempty"`
	IsPrivate  bool   `bson:"is_private" json:"is_private"`
}

// AccessPolicy defines global access and registration policy.
type AccessPolicy struct {
	RegistrationMode  string         `bson:"registration_mode" json:"registration_mode"` // "open", "invite", "allowlist", "closed"
	EnforceMembership bool           `bson:"enforce_membership" json:"enforce_membership"`
	LockMessage       string         `bson:"lock_message,omitempty" json:"lock_message,omitempty"`
	RequiredChats     []RequiredChat `bson:"required_chats" json:"required_chats"`
	UpdatedAt         float64        `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	UpdatedBy         int64          `bson:"updated_by,omitempty" json:"updated_by,omitempty"`
}

// PolicyPatch payload for updating AccessPolicy.
type PolicyPatch struct {
	RegistrationMode  *string         `json:"registration_mode,omitempty"`
	EnforceMembership *bool           `json:"enforce_membership,omitempty"`
	LockMessage       *string         `json:"lock_message,omitempty"`
	RequiredChats     *[]RequiredChat `json:"required_chats,omitempty"`
}

// InviteCode represents a registration invite code.
type InviteCode struct {
	Code      string   `bson:"_id" json:"code"`
	MaxUses   int      `bson:"max_uses" json:"max_uses"`
	UsedCount int      `bson:"used_count" json:"used_count"`
	CreatedBy int64    `bson:"created_by" json:"created_by"`
	CreatedAt float64  `bson:"created_at" json:"created_at"`
	ExpiresAt *float64 `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
	Note      string   `bson:"note,omitempty" json:"note,omitempty"`
	Revoked   bool     `bson:"revoked" json:"revoked"`
}

// InviteCreateRequest payload for POST /admin/access/invites.
type InviteCreateRequest struct {
	MaxUses int    `json:"max_uses"`
	TTLDays int    `json:"ttl_days"`
	Note    string `json:"note"`
}

// AllowlistEntry represents a whitelisted user in MongoDB allowlist.
type AllowlistEntry struct {
	UserID  int64   `bson:"_id" json:"user_id"`
	AddedBy int64   `bson:"added_by" json:"added_by"`
	AddedAt float64 `bson:"added_at" json:"added_at"`
	Note    string  `bson:"note,omitempty" json:"note,omitempty"`
}

// AllowlistRequest payload for POST /admin/access/allowlist and /bypass.
type AllowlistRequest struct {
	UserID int64  `json:"user_id"`
	Note   string `json:"note"`
}

// LockUserRequest payload for POST /admin/access/users/{id}/lock.
type LockUserRequest struct {
	Reason         string `json:"reason"`
	RevokeSessions bool   `json:"revoke_sessions"`
}
