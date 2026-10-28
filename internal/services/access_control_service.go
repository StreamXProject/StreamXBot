package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"streamgo/internal/models"
	"streamgo/internal/repository"
)

// AccessControlService handles access control policies, invites, allowlists, and user account state.
type AccessControlService struct {
	repo repository.AccessControlRepository
}

// NewAccessControlService creates a new AccessControlService.
func NewAccessControlService(repo repository.AccessControlRepository) *AccessControlService {
	return &AccessControlService{repo: repo}
}

// GetPolicy retrieves the active access policy.
func (s *AccessControlService) GetPolicy(ctx context.Context) (*models.AccessPolicy, error) {
	return s.repo.GetPolicy(ctx)
}

// UpdatePolicy applies partial updates to AccessPolicy.
func (s *AccessControlService) UpdatePolicy(ctx context.Context, patch models.PolicyPatch, adminID int64) (*models.AccessPolicy, error) {
	policy, err := s.repo.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}

	if patch.RegistrationMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*patch.RegistrationMode))
		if mode != "open" && mode != "invite" && mode != "allowlist" && mode != "closed" {
			return nil, errors.New("invalid registration_mode (must be open, invite, allowlist, closed)")
		}
		policy.RegistrationMode = mode
	}

	if patch.EnforceMembership != nil {
		policy.EnforceMembership = *patch.EnforceMembership
	}
	if patch.LockMessage != nil {
		policy.LockMessage = *patch.LockMessage
	}
	if patch.RequiredChats != nil {
		policy.RequiredChats = *patch.RequiredChats
	}

	policy.UpdatedAt = float64(time.Now().Unix())
	policy.UpdatedBy = adminID

	if err := s.repo.SavePolicy(ctx, policy); err != nil {
		return nil, err
	}
	return policy, nil
}

// AddRequiredChat adds a chat to required chats in policy.
func (s *AccessControlService) AddRequiredChat(ctx context.Context, chat models.RequiredChat) (*models.AccessPolicy, error) {
	policy, err := s.repo.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}

	for i, c := range policy.RequiredChats {
		if c.ChatID == chat.ChatID {
			policy.RequiredChats[i] = chat
			_ = s.repo.SavePolicy(ctx, policy)
			return policy, nil
		}
	}

	policy.RequiredChats = append(policy.RequiredChats, chat)
	if err := s.repo.SavePolicy(ctx, policy); err != nil {
		return nil, err
	}
	return policy, nil
}

// RemoveRequiredChat removes a chat from required chats in policy.
func (s *AccessControlService) RemoveRequiredChat(ctx context.Context, chatID int64) (*models.AccessPolicy, error) {
	policy, err := s.repo.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []models.RequiredChat
	for _, c := range policy.RequiredChats {
		if c.ChatID != chatID {
			filtered = append(filtered, c)
		}
	}
	policy.RequiredChats = filtered
	if err := s.repo.SavePolicy(ctx, policy); err != nil {
		return nil, err
	}
	return policy, nil
}

// CreateInvite generates a random invite code.
func (s *AccessControlService) CreateInvite(ctx context.Context, adminID int64, maxUses int, ttlDays int, note string) (*models.InviteCode, error) {
	if maxUses <= 0 {
		maxUses = 1
	}

	randBytes := make([]byte, 6)
	_, _ = rand.Read(randBytes)
	code := strings.ToUpper(hex.EncodeToString(randBytes))

	now := float64(time.Now().Unix())
	var expiresAt *float64
	if ttlDays > 0 {
		exp := now + float64(ttlDays*86400)
		expiresAt = &exp
	}

	inv := &models.InviteCode{
		Code:      code,
		MaxUses:   maxUses,
		UsedCount: 0,
		CreatedBy: adminID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Note:      note,
		Revoked:   false,
	}

	if err := s.repo.CreateInvite(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// RevokeInvite revokes an invite code.
func (s *AccessControlService) RevokeInvite(ctx context.Context, code string) bool {
	return s.repo.RevokeInvite(ctx, code)
}

// ListInvites lists invite codes.
func (s *AccessControlService) ListInvites(ctx context.Context, includeDead bool) ([]*models.InviteCode, error) {
	return s.repo.ListInvites(ctx, includeDead)
}

// AddToAllowlist adds a user ID to allowlist.
func (s *AccessControlService) AddToAllowlist(ctx context.Context, userID int64, adminID int64, note string) error {
	entry := &models.AllowlistEntry{
		UserID:  userID,
		AddedBy: adminID,
		AddedAt: float64(time.Now().Unix()),
		Note:    note,
	}
	return s.repo.AddToAllowlist(ctx, entry)
}

// RemoveFromAllowlist removes a user ID from allowlist.
func (s *AccessControlService) RemoveFromAllowlist(ctx context.Context, userID int64) bool {
	return s.repo.RemoveFromAllowlist(ctx, userID)
}

// ListAllowlist lists all allowlisted users.
func (s *AccessControlService) ListAllowlist(ctx context.Context) ([]*models.AllowlistEntry, error) {
	return s.repo.ListAllowlist(ctx)
}

// AddToBypass adds a user to bypass list.
func (s *AccessControlService) AddToBypass(ctx context.Context, userID int64, adminID int64, note string) error {
	entry := &models.AllowlistEntry{
		UserID:  userID,
		AddedBy: adminID,
		AddedAt: float64(time.Now().Unix()),
		Note:    note,
	}
	return s.repo.AddToBypass(ctx, entry)
}

// RemoveFromBypass removes a user from bypass list.
func (s *AccessControlService) RemoveFromBypass(ctx context.Context, userID int64) bool {
	return s.repo.RemoveFromBypass(ctx, userID)
}

// IsInBypass checks if a user is in the bypass list.
func (s *AccessControlService) IsInBypass(ctx context.Context, userID int64) bool {
	return s.repo.IsInBypass(ctx, userID)
}

// ListBypass lists bypass entries.
func (s *AccessControlService) ListBypass(ctx context.Context) ([]*models.AllowlistEntry, error) {
	return s.repo.ListBypass(ctx)
}

// ListUsers lists user accounts for administration.
func (s *AccessControlService) ListUsers(ctx context.Context, query string, status string, limit int, skip int) (map[string]any, error) {
	if limit <= 0 {
		limit = 50
	}
	users, total, err := s.repo.ListUsers(ctx, query, status, limit, skip)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"users": users,
		"total": total,
		"limit": limit,
		"skip":  skip,
	}, nil
}

// LockUser locks a user account and optionally revokes active sessions.
func (s *AccessControlService) LockUser(ctx context.Context, userID int64, reason string, adminID int64, revokeSessions bool) error {
	if err := s.repo.SetUserStatus(ctx, userID, "locked", reason); err != nil {
		return err
	}
	if revokeSessions {
		_, _ = s.repo.RevokeUserSessions(ctx, userID)
	}
	return nil
}

// UnlockUser unlocks a user account.
func (s *AccessControlService) UnlockUser(ctx context.Context, userID int64) error {
	return s.repo.SetUserStatus(ctx, userID, "active", "")
}

// RevokeSessions revokes all active tokens for a user.
func (s *AccessControlService) RevokeSessions(ctx context.Context, userID int64) (int, error) {
	return s.repo.RevokeUserSessions(ctx, userID)
}

// AssertCanRegister checks registration policy rules against user ID and invite code.
func (s *AccessControlService) AssertCanRegister(ctx context.Context, userID int64, inviteCode string) error {
	policy, err := s.repo.GetPolicy(ctx)
	if err != nil {
		return nil // Fallback open if policy check errors
	}

	switch policy.RegistrationMode {
	case "closed":
		return errors.New("registration_closed: New accounts cannot be created at this time")
	case "allowlist":
		if userID > 0 && !s.repo.IsInAllowlist(ctx, userID) {
			return errors.New("not_allowlisted: Your account is not on the registration allowlist")
		}
	case "invite":
		code := strings.ToUpper(strings.TrimSpace(inviteCode))
		if code == "" {
			return errors.New("invite_required: An invite code is required to register")
		}
		inv, err := s.repo.GetInvite(ctx, code)
		if err != nil || inv == nil || inv.Revoked {
			return errors.New("invite_invalid: That invite code is invalid or has been revoked")
		}
		if inv.ExpiresAt != nil && float64(time.Now().Unix()) > *inv.ExpiresAt {
			return errors.New("invite_expired: That invite code has expired")
		}
		if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
			return errors.New("invite_exhausted: That invite code has already reached its maximum uses")
		}
		_ = s.repo.ConsumeInvite(ctx, code)
	}

	return nil
}
