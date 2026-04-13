package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	coreauth "github.com/ismd/linktheca/internal/core/auth"
)

var (
	ErrRegistrationDisabled = errors.New("registration disabled")
	ErrWeakPassword         = errors.New("password too short")
	ErrInvalidCredentials   = errors.New("invalid credentials")
)

const minPasswordLen = 10

type StoreAPI interface {
	CreateUser(ctx context.Context, email, passwordHash, displayName string, isAdmin bool) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id int64) (*User, error)
	CountUsers(ctx context.Context) (int64, error)
	CreateRefreshToken(ctx context.Context, userID int64, tokenHash, userAgent string, expiresAt time.Time) (*RefreshToken, error)
	FindActiveRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id int64) error
}

type ServiceConfig struct {
	RefreshTTL          time.Duration
	RegistrationEnabled bool
}

type Service struct {
	store StoreAPI
	jwt   *coreauth.JWTIssuer
	cfg   ServiceConfig
}

func NewService(store StoreAPI, jwt *coreauth.JWTIssuer, cfg ServiceConfig) *Service {
	return &Service{store: store, jwt: jwt, cfg: cfg}
}

// Register creates a new user account and returns tokens plus the user record
// The first user created on the instance is promoted to admin
func (s *Service) Register(ctx context.Context, req RegisterRequest, userAgent string) (*AuthResponse, error) {
	if !s.cfg.RegistrationEnabled {
		return nil, ErrRegistrationDisabled
	}
	if len(req.Password) < minPasswordLen {
		return nil, ErrWeakPassword
	}

	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	isAdmin := count == 0

	hash, err := coreauth.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.store.CreateUser(ctx, req.Email, hash, req.DisplayName, isAdmin)
	if err != nil {
		return nil, err
	}

	tokens, err := s.issueTokens(ctx, user, userAgent)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{User: *user, Tokens: *tokens}, nil
}

// Refresh exchanges a valid refresh token for a new access+refresh pair
// Implements rotation: the old refresh is revoked immediately
func (s *Service) Refresh(ctx context.Context, req RefreshRequest, userAgent string) (*AuthResponse, error) {
	hash := coreauth.HashRefreshToken(req.RefreshToken)
	existing, err := s.store.FindActiveRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	user, err := s.store.GetUserByID(ctx, existing.UserID)
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}

	// Revoke the old token first. Even if token issuance fails, the old one is dead.
	if err := s.store.RevokeRefreshToken(ctx, existing.ID); err != nil {
		return nil, fmt.Errorf("revoke previous: %w", err)
	}

	tokens, err := s.issueTokens(ctx, user, userAgent)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{User: *user, Tokens: *tokens}, nil
}

// Logout revokes the provided refresh token. Idempotent: unknown or already-revoked tokens return nil (we do not leak whether the token existed)
func (s *Service) Logout(ctx context.Context, req RefreshRequest) error {
	hash := coreauth.HashRefreshToken(req.RefreshToken)
	existing, err := s.store.FindActiveRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	return s.store.RevokeRefreshToken(ctx, existing.ID)
}

// issueTokens creates an access token and a refresh token record
func (s *Service) issueTokens(ctx context.Context, user *User, userAgent string) (*TokenPair, error) {
	access, err := s.jwt.Issue(user.ID, user.IsAdmin)
	if err != nil {
		return nil, fmt.Errorf("issue access: %w", err)
	}

	refresh, err := coreauth.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh: %w", err)
	}

	_, err = s.store.CreateRefreshToken(ctx,
		user.ID,
		coreauth.HashRefreshToken(refresh),
		userAgent,
		time.Now().Add(s.cfg.RefreshTTL),
	)
	if err != nil {
		return nil, fmt.Errorf("persist refresh: %w", err)
	}

	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

// Login authenticates a user by email and password
// Returns ErrInvalidCredentials for any authentication failure (do not leak whether the email exists)
func (s *Service) Login(ctx context.Context, req LoginRequest, userAgent string) (*AuthResponse, error) {
	user, err := s.store.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	ok, err := coreauth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !ok {
		return nil, ErrInvalidCredentials
	}

	tokens, err := s.issueTokens(ctx, user, userAgent)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{User: *user, Tokens: *tokens}, nil
}
