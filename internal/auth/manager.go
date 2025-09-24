package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RolePlayer Role = "player"
)

var (
	ErrAccountExists      = errors.New("account already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrForbidden          = errors.New("forbidden")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrInvalidUsername    = errors.New("username is required")
	ErrInvalidPassword    = errors.New("password is required")
	ErrAccountNotFound    = errors.New("account not found")
)

type Account struct {
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Role        Role      `json:"role"`
	Agency      string    `json:"agency"`
	CreatedAt   time.Time `json:"createdAt"`
	CreatedBy   string    `json:"createdBy,omitempty"`
}

type AccountRecord struct {
	Account
	PasswordHash []byte
}

type AccountRepository interface {
	CreateAccount(ctx context.Context, record *AccountRecord) (*Account, error)
	FindByUsername(ctx context.Context, username string) (*AccountRecord, error)
	ListAccounts(ctx context.Context, role Role) ([]Account, error)
}

type TokenStore interface {
	SaveToken(ctx context.Context, token string, subject string, ttl time.Duration) error
	DeleteToken(ctx context.Context, token string) error
	LookupSubject(ctx context.Context, token string) (string, error)
}

type Manager struct {
	repo      AccountRepository
	tokens    TokenStore
	jwtSecret []byte
	jwtIssuer string
	tokenTTL  time.Duration
}

func NewManager(repo AccountRepository, tokenStore TokenStore, jwtSecret, jwtIssuer string, ttl time.Duration) (*Manager, error) {
	if repo == nil {
		return nil, errors.New("auth manager: repository is required")
	}
	if tokenStore == nil {
		return nil, errors.New("auth manager: token store is required")
	}
	secret := strings.TrimSpace(jwtSecret)
	if secret == "" {
		return nil, errors.New("auth manager: jwt secret is required")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	manager := &Manager{
		repo:      repo,
		tokens:    tokenStore,
		jwtSecret: []byte(secret),
		jwtIssuer: strings.TrimSpace(jwtIssuer),
		tokenTTL:  ttl,
	}
	if err := manager.ensureBootstrapAdmin(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) ensureBootstrapAdmin() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.repo.FindByUsername(ctx, "admin01"); err == nil {
		return nil
	} else if !errors.Is(err, ErrAccountNotFound) {
		return err
	}
	passwordHash := hashPassword("admin01pass")
	_, err := m.repo.CreateAccount(ctx, &AccountRecord{
		Account: Account{
			Username:    "admin01",
			DisplayName: "客服主管",
			Role:        RoleAdmin,
			Agency:      "master",
			CreatedBy:   "system",
		},
		PasswordHash: passwordHash[:],
	})
	return err
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func (m *Manager) CreateAccount(ctx context.Context, creator string, role Role, agency, username, password, displayName string) (*Account, error) {
	username = normalizeUsername(username)
	if username == "" {
		return nil, ErrInvalidUsername
	}
	if strings.TrimSpace(password) == "" {
		return nil, ErrInvalidPassword
	}
	creator = normalizeUsername(creator)
	if displayName == "" {
		displayName = username
	}
	if agency = strings.TrimSpace(agency); agency == "" {
		agency = "default"
	}

	if role == RoleAdmin {
		if creator != "admin01" {
			return nil, ErrForbidden
		}
	} else {
		if creator != "" && creator != username {
			creatorAccount, err := m.repo.FindByUsername(ctx, creator)
			if err != nil {
				if errors.Is(err, ErrAccountNotFound) {
					return nil, ErrForbidden
				}
				return nil, err
			}
			if creatorAccount.Role != RoleAdmin {
				return nil, ErrForbidden
			}
		}
	}

	createdBy := creator
	if createdBy == "" {
		createdBy = username
	}

	passwordHash := hashPassword(password)
	account := &AccountRecord{
		Account: Account{
			Username:    username,
			DisplayName: displayName,
			Role:        role,
			Agency:      agency,
			CreatedBy:   createdBy,
			CreatedAt:   time.Now(),
		},
		PasswordHash: passwordHash[:],
	}
	created, err := m.repo.CreateAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (m *Manager) Login(ctx context.Context, username, password string) (string, *Account, error) {
	username = normalizeUsername(username)
	if username == "" || strings.TrimSpace(password) == "" {
		return "", nil, ErrInvalidCredentials
	}
	record, err := m.repo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, err
	}
	if !verifyPassword(record.PasswordHash, password) {
		return "", nil, ErrInvalidCredentials
	}
	token, err := m.generateToken(record.Account)
	if err != nil {
		return "", nil, err
	}
	if err := m.tokens.SaveToken(ctx, token, record.Username, m.tokenTTL); err != nil {
		return "", nil, err
	}
	account := record.Account
	return token, &account, nil
}

func (m *Manager) generateToken(account Account) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":    account.Username,
		"role":   string(account.Role),
		"agency": account.Agency,
		"iat":    now.Unix(),
		"exp":    now.Add(m.tokenTTL).Unix(),
	}
	if m.jwtIssuer != "" {
		claims["iss"] = m.jwtIssuer
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.jwtSecret)
}

func (m *Manager) Authenticate(ctx context.Context, token string) (*Account, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrUnauthorized
	}
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrUnauthorized
		}
		return m.jwtSecret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, ErrUnauthorized
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrUnauthorized
	}
	if issuer, ok := claims["iss"].(string); ok && m.jwtIssuer != "" && issuer != m.jwtIssuer {
		return nil, ErrUnauthorized
	}
	subject, _ := claims["sub"].(string)
	if subject == "" {
		return nil, ErrUnauthorized
	}
	storedSubject, err := m.tokens.LookupSubject(ctx, token)
	if err != nil || storedSubject != subject {
		return nil, ErrUnauthorized
	}
	record, err := m.repo.FindByUsername(ctx, subject)
	if err != nil {
		return nil, ErrUnauthorized
	}
	account := record.Account
	return &account, nil
}

func (m *Manager) Logout(ctx context.Context, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	_ = m.tokens.DeleteToken(ctx, token)
}

func (m *Manager) Account(ctx context.Context, username string) (*Account, error) {
	record, err := m.repo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	account := record.Account
	return &account, nil
}

func (m *Manager) ListAccounts(ctx context.Context, role Role) ([]Account, error) {
	return m.repo.ListAccounts(ctx, role)
}

func hashPassword(password string) [32]byte {
	return sha256.Sum256([]byte(password))
}

func verifyPassword(hash []byte, password string) bool {
	if len(hash) != 32 {
		return false
	}
	candidate := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(hash, candidate[:]) == 1
}

func EncodePasswordHash(hash [32]byte) string {
	return base64.StdEncoding.EncodeToString(hash[:])
}

func DecodePasswordHash(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}
