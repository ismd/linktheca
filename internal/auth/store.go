package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when the requested row does not exist.
var ErrNotFound = errors.New("not found")

// ErrEmailTaken is returned when attempting to create a user with an existing email.
var ErrEmailTaken = errors.New("email already registered")

// Store provides persistence for users and refresh tokens.
type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) CreateUser(ctx context.Context, email, passwordHash, displayName string, isAdmin bool) (*User, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name, is_admin)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, password_hash, display_name, is_admin, created_at, updated_at
	`, email, passwordHash, displayName, isAdmin)

	u, err := scanUser(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	return u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, email, password_hash, display_name, is_admin, created_at, updated_at
		FROM users
		WHERE email = $1
	`, email)

	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	return u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, email, password_hash, display_name, is_admin, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id)

	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return u, nil
}

func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	var n int64

	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return n, nil
}

func scanUser(row pgx.Row) (*User, error) {
	var u User

	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &u, nil
}

func (s *Store) CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (*RefreshToken, error) {
	var rt RefreshToken

	err := s.db.QueryRow(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, expires_at, revoked_at, created_at
	`, userID, tokenHash, expiresAt).Scan(
		&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.RevokedAt, &rt.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}

	return &rt, nil
}

func (s *Store) FindActiveRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var rt RefreshToken

	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
	`, tokenHash).Scan(
		&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.RevokedAt, &rt.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("find refresh token: %w", err)
	}

	return &rt, nil
}

func (s *Store) RevokeRefreshToken(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL
	`, id)

	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}

	return nil
}
