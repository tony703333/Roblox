package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQL struct {
	DB *sql.DB
}

func NewMySQL(dsn string) (*MySQL, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	store := &MySQL{DB: db}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (m *MySQL) Close() error {
	if m.DB == nil {
		return nil
	}
	return m.DB.Close()
}

func (m *MySQL) migrate(ctx context.Context) error {
	if m.DB == nil {
		return errors.New("mysql: db is nil")
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
            id BIGINT AUTO_INCREMENT PRIMARY KEY,
            username VARCHAR(191) NOT NULL UNIQUE,
            display_name VARCHAR(255) NOT NULL,
            role VARCHAR(32) NOT NULL,
            agency VARCHAR(64) NOT NULL,
            password_hash VARBINARY(64) NOT NULL,
            created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
            created_by VARCHAR(191)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS agency_settings (
            id BIGINT AUTO_INCREMENT PRIMARY KEY,
            agency VARCHAR(64) NOT NULL UNIQUE,
            charge_api TEXT,
            withdraw_api TEXT,
            bet_api TEXT,
            player_info_api TEXT,
            updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	for _, stmt := range statements {
		if _, err := m.DB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("mysql migrate: %w", err)
		}
	}
	return nil
}
