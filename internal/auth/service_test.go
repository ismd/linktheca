package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/auth"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

type mockStore struct {
	users         map[string]*auth.User
	usersByID     map[int64]*auth.User
	refreshByHash map[string]*auth.RefreshToken
	nextUserID    int64
	nextRTID      int64
	countErr      error
}

func newMockStore() *mockStore {
	return &mockStore{
		users:         make(map[string]*auth.User),
		usersByID:     make(map[int64]*auth.User),
		refreshByHash: make(map[string]*auth.RefreshToken),
	}
}

func (m *mockStore) CreateUser(_ context.Context, email, hash, name string, isAdmin bool) (*auth.User, error) {
	if _, ok := m.users[email]; ok {
		return nil, auth.ErrEmailTaken
	}

	m.nextUserID++
	u := &auth.User{
		ID: m.nextUserID, Email: email, PasswordHash: hash, DisplayName: name, IsAdmin: isAdmin,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	m.users[email] = u
	m.usersByID[u.ID] = u

	return u, nil
}

func (m *mockStore) GetUserByEmail(_ context.Context, email string) (*auth.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, auth.ErrNotFound
	}

	return u, nil
}

func (m *mockStore) GetUserByID(_ context.Context, id int64) (*auth.User, error) {
	u, ok := m.usersByID[id]
	if !ok {
		return nil, auth.ErrNotFound
	}

	return u, nil
}

func (m *mockStore) CountUsers(_ context.Context) (int64, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}

	return int64(len(m.users)), nil
}

func (m *mockStore) CreateRefreshToken(_ context.Context, userID int64, hash, ua string, exp time.Time) (*auth.RefreshToken, error) {
	m.nextRTID++
	rt := &auth.RefreshToken{
		ID: m.nextRTID, UserID: userID, TokenHash: hash, ExpiresAt: exp, UserAgent: ua, CreatedAt: time.Now(),
	}
	m.refreshByHash[hash] = rt

	return rt, nil
}

func (m *mockStore) FindActiveRefreshToken(_ context.Context, hash string) (*auth.RefreshToken, error) {
	rt, ok := m.refreshByHash[hash]
	if !ok {
		return nil, auth.ErrNotFound
	}

	if rt.RevokedAt != nil {
		return nil, auth.ErrNotFound
	}

	if time.Now().After(rt.ExpiresAt) {
		return nil, auth.ErrNotFound
	}

	return rt, nil
}

func (m *mockStore) RevokeRefreshToken(_ context.Context, id int64) error {
	for _, rt := range m.refreshByHash {
		if rt.ID == id {
			now := time.Now()
			rt.RevokedAt = &now
			return nil
		}
	}

	return nil
}

func newTestService(t *testing.T, store auth.StoreAPI, registration bool) *auth.Service {
	t.Helper()
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)

	return auth.NewService(store, issuer, auth.ServiceConfig{
		RefreshTTL:          720 * time.Hour,
		RegistrationEnabled: registration,
	})
}

func TestRegisterFirstUserBecomesAdmin(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	resp, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "root@example.com", Password: "a-strong-password", DisplayName: "Root",
	}, "ua")

	require.NoError(t, err)
	require.True(t, resp.User.IsAdmin)
	require.NotEmpty(t, resp.Tokens.AccessToken)
	require.NotEmpty(t, resp.Tokens.RefreshToken)
}

func TestRegisterSecondUserIsNotAdmin(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "a@example.com", Password: "a-strong-password", DisplayName: "A",
	}, "ua")
	require.NoError(t, err)

	resp, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "b@example.com", Password: "another-strong-password", DisplayName: "B",
	}, "ua")

	require.NoError(t, err)
	require.False(t, resp.User.IsAdmin)
}

func TestRegisterDisabledReturnsError(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, false)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "a@example.com", Password: "a-strong-password", DisplayName: "A",
	}, "ua")

	require.ErrorIs(t, err, auth.ErrRegistrationDisabled)
}

func TestRegisterShortPassword(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "a@example.com", Password: "short", DisplayName: "A",
	}, "ua")

	require.ErrorIs(t, err, auth.ErrWeakPassword)
}

func TestRegisterDuplicateEmail(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "dup@example.com", Password: "a-strong-password", DisplayName: "Dup",
	}, "ua")
	require.NoError(t, err)

	_, err = svc.Register(context.Background(), auth.RegisterRequest{
		Email: "dup@example.com", Password: "another-password", DisplayName: "Dup2",
	}, "ua")
	require.ErrorIs(t, err, auth.ErrEmailTaken)
}

// Sanity: interface compile-time check
var _ auth.StoreAPI = (*mockStore)(nil)
var _ = errors.New
