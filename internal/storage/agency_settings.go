package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type AgencyAPISettings struct {
	Agency        string    `json:"agency"`
	ChargeAPI     string    `json:"chargeApi"`
	WithdrawAPI   string    `json:"withdrawApi"`
	BetAPI        string    `json:"betApi"`
	PlayerInfoAPI string    `json:"playerInfoApi"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type AgencySettingsStore interface {
	List(ctx context.Context) ([]AgencyAPISettings, error)
	Get(ctx context.Context, agency string) (*AgencyAPISettings, error)
	Upsert(ctx context.Context, settings *AgencyAPISettings) error
}

type AgencySettingsRepository struct {
	db *sql.DB
}

func NewAgencySettingsRepository(db *sql.DB) *AgencySettingsRepository {
	return &AgencySettingsRepository{db: db}
}

func (r *AgencySettingsRepository) List(ctx context.Context) ([]AgencyAPISettings, error) {
	if r.db == nil {
		return nil, errors.New("agency settings repository: db is nil")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT agency, charge_api, withdraw_api, bet_api, player_info_api, updated_at FROM agency_settings ORDER BY agency`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AgencyAPISettings
	for rows.Next() {
		var settings AgencyAPISettings
		if err := rows.Scan(&settings.Agency, &settings.ChargeAPI, &settings.WithdrawAPI, &settings.BetAPI, &settings.PlayerInfoAPI, &settings.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, settings)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *AgencySettingsRepository) Get(ctx context.Context, agency string) (*AgencyAPISettings, error) {
	if r.db == nil {
		return nil, errors.New("agency settings repository: db is nil")
	}
	normalized := strings.TrimSpace(strings.ToLower(agency))
	row := r.db.QueryRowContext(ctx, `SELECT agency, charge_api, withdraw_api, bet_api, player_info_api, updated_at FROM agency_settings WHERE agency = ? LIMIT 1`, normalized)
	var settings AgencyAPISettings
	if err := row.Scan(&settings.Agency, &settings.ChargeAPI, &settings.WithdrawAPI, &settings.BetAPI, &settings.PlayerInfoAPI, &settings.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &settings, nil
}

func (r *AgencySettingsRepository) Upsert(ctx context.Context, settings *AgencyAPISettings) error {
	if r.db == nil {
		return errors.New("agency settings repository: db is nil")
	}
	normalized := strings.TrimSpace(strings.ToLower(settings.Agency))
	if normalized == "" {
		return errors.New("agency is required")
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO agency_settings (agency, charge_api, withdraw_api, bet_api, player_info_api)
        VALUES (?, ?, ?, ?, ?)
        ON DUPLICATE KEY UPDATE
            charge_api = VALUES(charge_api),
            withdraw_api = VALUES(withdraw_api),
            bet_api = VALUES(bet_api),
            player_info_api = VALUES(player_info_api)`,
		normalized,
		settings.ChargeAPI,
		settings.WithdrawAPI,
		settings.BetAPI,
		settings.PlayerInfoAPI,
	)
	return err
}
