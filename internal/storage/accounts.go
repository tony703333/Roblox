package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	"im/internal/auth"
)

type AccountRepository struct {
	db *sql.DB
}

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) CreateAccount(ctx context.Context, record *auth.AccountRecord) (*auth.Account, error) {
	if r.db == nil {
		return nil, errors.New("account repository: db is nil")
	}
	account := record.Account
	now := time.Now().UTC()
	if account.CreatedAt.IsZero() {
		account.CreatedAt = now
	}
	account.Username = strings.ToLower(strings.TrimSpace(account.Username))
	if account.DisplayName == "" {
		account.DisplayName = account.Username
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO accounts (username, display_name, role, agency, password_hash, created_at, created_by)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		account.Username,
		account.DisplayName,
		string(account.Role),
		account.Agency,
		record.PasswordHash,
		account.CreatedAt,
		account.CreatedBy,
	)
	if err != nil {
		if isDuplicateErr(err) {
			return nil, auth.ErrAccountExists
		}
		return nil, err
	}
	result := account
	return &result, nil
}

func (r *AccountRepository) FindByUsername(ctx context.Context, username string) (*auth.AccountRecord, error) {
	if r.db == nil {
		return nil, errors.New("account repository: db is nil")
	}
	normalized := strings.ToLower(strings.TrimSpace(username))
	row := r.db.QueryRowContext(ctx, `SELECT username, display_name, role, agency, password_hash, created_at, created_by FROM accounts WHERE username = ? LIMIT 1`, normalized)
	var record auth.AccountRecord
	var role string
	var createdBy sql.NullString
	if err := row.Scan(&record.Username, &record.DisplayName, &role, &record.Agency, &record.PasswordHash, &record.CreatedAt, &createdBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, auth.ErrAccountNotFound
		}
		return nil, err
	}
	record.Role = auth.Role(role)
	if createdBy.Valid {
		record.CreatedBy = createdBy.String
	}
	return &record, nil
}

func (r *AccountRepository) ListAccounts(ctx context.Context, role auth.Role) ([]auth.Account, error) {
	if r.db == nil {
		return nil, errors.New("account repository: db is nil")
	}
	var rows *sql.Rows
	var err error
	if role != "" {
		rows, err = r.db.QueryContext(ctx, `SELECT username, display_name, role, agency, created_at, created_by FROM accounts WHERE role = ? ORDER BY username`, string(role))
	} else {
		rows, err = r.db.QueryContext(ctx, `SELECT username, display_name, role, agency, created_at, created_by FROM accounts ORDER BY username`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []auth.Account
	for rows.Next() {
		var account auth.Account
		var roleValue string
		var createdBy sql.NullString
		if err := rows.Scan(&account.Username, &account.DisplayName, &roleValue, &account.Agency, &account.CreatedAt, &createdBy); err != nil {
			return nil, err
		}
		account.Role = auth.Role(roleValue)
		if createdBy.Valid {
			account.CreatedBy = createdBy.String
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return accounts, nil
}

func isDuplicateErr(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}
