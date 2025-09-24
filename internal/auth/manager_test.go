package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryAccountRepo struct {
	mu       sync.RWMutex
	accounts map[string]*AccountRecord
}

func newMemoryAccountRepo() *memoryAccountRepo {
	repo := &memoryAccountRepo{accounts: make(map[string]*AccountRecord)}
	return repo
}

func (r *memoryAccountRepo) CreateAccount(ctx context.Context, record *AccountRecord) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	username := normalizeUsername(record.Username)
	if username == "" {
		return nil, ErrInvalidUsername
	}
	if _, exists := r.accounts[username]; exists {
		return nil, ErrAccountExists
	}
	clone := *record
	clone.Account = record.Account
	clone.Username = username
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = time.Now()
	}
	r.accounts[username] = &clone
	account := clone.Account
	return &account, nil
}

func (r *memoryAccountRepo) FindByUsername(ctx context.Context, username string) (*AccountRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	normalized := normalizeUsername(username)
	account, ok := r.accounts[normalized]
	if !ok {
		return nil, ErrAccountNotFound
	}
	clone := *account
	clone.Account = account.Account
	clone.PasswordHash = append([]byte(nil), account.PasswordHash...)
	return &clone, nil
}

func (r *memoryAccountRepo) ListAccounts(ctx context.Context, role Role) ([]Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Account, 0, len(r.accounts))
	for _, record := range r.accounts {
		if role != "" && record.Role != role {
			continue
		}
		result = append(result, record.Account)
	}
	return result, nil
}

type memoryTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]string
}

func newMemoryTokenStore() *memoryTokenStore {
	return &memoryTokenStore{tokens: make(map[string]string)}
}

func (s *memoryTokenStore) SaveToken(ctx context.Context, token, subject string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = subject
	return nil
}

func (s *memoryTokenStore) DeleteToken(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
	return nil
}

func (s *memoryTokenStore) LookupSubject(ctx context.Context, token string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	subject, ok := s.tokens[token]
	if !ok {
		return "", errors.New("token missing")
	}
	return subject, nil
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	repo := newMemoryAccountRepo()
	tokens := newMemoryTokenStore()
	manager, err := NewManager(repo, tokens, "test-secret", "test-suite", time.Hour)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return manager
}

func TestBootstrapAdmin(t *testing.T) {
	mgr := newTestManager(t)
	account, err := mgr.Account(context.Background(), "admin01")
	if err != nil {
		t.Fatalf("expected admin01 to exist: %v", err)
	}
	if account.Role != RoleAdmin {
		t.Fatalf("expected admin01 to be admin, got %s", account.Role)
	}
	if account.Agency == "" {
		t.Fatalf("expected admin agency to be set")
	}
}

func TestCreateAccountsAndLogin(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	player, err := mgr.CreateAccount(ctx, "", RolePlayer, "agency-a", "player01", "secret", "玩家01")
	if err != nil {
		t.Fatalf("create player failed: %v", err)
	}
	if player.Role != RolePlayer {
		t.Fatalf("expected role player, got %s", player.Role)
	}
	if player.Agency != "agency-a" {
		t.Fatalf("expected agency to persist")
	}

	if _, err := mgr.CreateAccount(ctx, "player01", RoleAdmin, "master", "admin02", "pass", "管理員"); err == nil {
		t.Fatalf("expected non-admin to be forbidden")
	}

	if _, err := mgr.CreateAccount(ctx, "admin01", RoleAdmin, "master", "admin02", "pass", "客服02"); err != nil {
		t.Fatalf("expected admin01 to create admin: %v", err)
	}

	token, _, err := mgr.Login(ctx, "admin02", "pass")
	if err != nil {
		t.Fatalf("login admin02 failed: %v", err)
	}
	mgr.Logout(ctx, token)
	if _, err := mgr.CreateAccount(ctx, "admin02", RoleAdmin, "master", "admin03", "pass", "客服03"); err == nil {
		t.Fatalf("expected admin02 to be forbidden to create admin")
	}

	if _, err := mgr.CreateAccount(ctx, "admin02", RolePlayer, "agency-b", "player02", "secret", "玩家02"); err != nil {
		t.Fatalf("expected admin02 to create player: %v", err)
	}

	token, account, err := mgr.Login(ctx, "player01", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if account.Username != "player01" {
		t.Fatalf("unexpected username: %s", account.Username)
	}

	authed, err := mgr.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if authed.Username != account.Username {
		t.Fatalf("expected same account after auth")
	}

	mgr.Logout(ctx, token)
	if _, err := mgr.Authenticate(ctx, token); err == nil {
		t.Fatalf("expected auth to fail after logout")
	}
}
