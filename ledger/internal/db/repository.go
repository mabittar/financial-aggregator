package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/financial-aggregator/ledger/internal/auth"
)

type UserRepository struct {
	conn *sql.DB
}

func NewUserRepository(conn *sql.DB) *UserRepository {
	return &UserRepository{conn: conn}
}

func (u *UserRepository) Create(ctx context.Context, user *auth.User) error {
	if user == nil {
		return fmt.Errorf("user is required")
	}

	query := `INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id, created_at`
	row := u.conn.QueryRowContext(ctx, query, user.Email, user.PasswordHash, user.DisplayName)
	if err := row.Scan(&user.ID, &user.CreatedAt); err != nil {
		return err
	}
	return nil
}

func (u *UserRepository) FindByEmail(ctx context.Context, email string) (*auth.User, error) {
	query := `SELECT id, email, password_hash, display_name, created_at FROM users WHERE email = $1`
	row := u.conn.QueryRowContext(ctx, query, email)
	var user auth.User
	var displayName sql.NullString
	if err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &displayName, &user.CreatedAt); err != nil {
		return nil, err
	}
	if displayName.Valid {
		user.DisplayName = displayName.String
	}
	return &user, nil
}

type Repository struct {
	conn *sql.DB
}

func NewRepository(conn *sql.DB) *Repository {
	return &Repository{conn: conn}
}

type Portfolio struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Holding struct {
	ID           string  `json:"id"`
	PortfolioID  string  `json:"portfolio_id"`
	AssetSymbol  string  `json:"asset_symbol"`
	Quantity     float64 `json:"quantity"`
	AveragePrice float64 `json:"average_price"`
}

type Transaction struct {
	ID         string    `json:"id"`
	HoldingID  string    `json:"holding_id"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	ExecutedAt time.Time `json:"executed_at"`
}

type IdempotencyKey struct {
	Key              string          `json:"key"`
	UserID           string          `json:"user_id"`
	PayloadHash      string          `json:"payload_hash"`
	ResponseMetadata json.RawMessage `json:"response_metadata"`
}

func (r *Repository) CreatePortfolio(ctx context.Context, userID, name string) (string, error) {
	var id string
	query := `INSERT INTO portfolios (user_id, name) VALUES ($1, $2) RETURNING id`
	if err := r.conn.QueryRowContext(ctx, query, userID, name).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) ListPortfolios(ctx context.Context, userID string) ([]Portfolio, error) {
	query := `SELECT id, name FROM portfolios WHERE user_id = $1 ORDER BY name`
	rows, err := r.conn.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var portfolios []Portfolio
	for rows.Next() {
		var p Portfolio
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, err
		}
		portfolios = append(portfolios, p)
	}
	return portfolios, rows.Err()
}

func (r *Repository) ListHoldings(ctx context.Context, userID string) ([]Holding, error) {
	query := `SELECT id, portfolio_id, asset_symbol, quantity, average_price FROM holdings WHERE user_id = $1 ORDER BY asset_symbol`
	rows, err := r.conn.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Holding
	for rows.Next() {
		var h Holding
		if err := rows.Scan(&h.ID, &h.PortfolioID, &h.AssetSymbol, &h.Quantity, &h.AveragePrice); err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

func (r *Repository) ListHoldingTransactions(ctx context.Context, userID, holdingID string) ([]Transaction, error) {
	query := `SELECT id, holding_id, amount, currency, executed_at FROM transactions WHERE holding_id = $1 AND user_id = $2 ORDER BY executed_at DESC`
	rows, err := r.conn.QueryContext(ctx, query, holdingID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.HoldingID, &t.Amount, &t.Currency, &t.ExecutedAt); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (r *Repository) FindIdempotencyKey(ctx context.Context, tx *sql.Tx, key string) (*IdempotencyKey, error) {
	query := `SELECT key, user_id, payload_hash, response_metadata FROM idempotency_keys WHERE key = $1`
	row := tx.QueryRowContext(ctx, query, key)
	var item IdempotencyKey
	if err := row.Scan(&item.Key, &item.UserID, &item.PayloadHash, &item.ResponseMetadata); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpsertIdempotencyKey(ctx context.Context, tx *sql.Tx, key, userID, payloadHash string, responseMetadata json.RawMessage) error {
	query := `INSERT INTO idempotency_keys (key, user_id, payload_hash, response_metadata) VALUES ($1, $2, $3, $4)
	ON CONFLICT (key) DO UPDATE SET payload_hash = EXCLUDED.payload_hash, response_metadata = EXCLUDED.response_metadata, updated_at = now()`
	_, err := tx.ExecContext(ctx, query, key, userID, payloadHash, responseMetadata)
	return err
}

func (r *Repository) InsertMonthlyStatement(ctx context.Context, tx *sql.Tx, userID, portfolioName string, referenceDate time.Time, ingestKey string, rawPayload, parsedPayload json.RawMessage, source string) (string, error) {
	query := `INSERT INTO monthly_statements (user_id, portfolio_name, reference_date, ingest_key, raw_payload, parsed_payload, status, source)
	VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7) RETURNING id`
	var id string
	if err := tx.QueryRowContext(ctx, query, userID, portfolioName, referenceDate, ingestKey, rawPayload, parsedPayload, source).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) UpdateMonthlyStatement(ctx context.Context, tx *sql.Tx, userID, portfolioName string, referenceDate time.Time, ingestKey string, rawPayload, parsedPayload json.RawMessage, source string) (string, error) {
	query := `UPDATE monthly_statements SET raw_payload = $1, parsed_payload = $2, status = 'pending', updated_at = now(), source = $3
	WHERE user_id = $4 AND portfolio_name = $5 AND reference_date = $6 AND ingest_key = $7 RETURNING id`
	var id string
	if err := tx.QueryRowContext(ctx, query, rawPayload, parsedPayload, source, userID, portfolioName, referenceDate, ingestKey).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) ListStatementsForPortfolio(ctx context.Context, userID, portfolioName string, referenceDate time.Time) ([]map[string]any, error) {
	query := `SELECT id, ingest_key, status, submitted_at, updated_at, source FROM monthly_statements
	WHERE user_id = $1 AND portfolio_name = $2 AND reference_date = $3 ORDER BY submitted_at DESC`
	rows, err := r.conn.QueryContext(ctx, query, userID, portfolioName, referenceDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var statements []map[string]any
	for rows.Next() {
		var id, ingestKey, status, source string
		var submittedAt, updatedAt time.Time
		if err := rows.Scan(&id, &ingestKey, &status, &submittedAt, &updatedAt, &source); err != nil {
			return nil, err
		}
		statements = append(statements, map[string]any{
			"id":           id,
			"ingest_key":   ingestKey,
			"status":       status,
			"submitted_at": submittedAt,
			"updated_at":   updatedAt,
			"source":       source,
		})
	}
	return statements, rows.Err()
}

func (r *Repository) ConfirmStatements(ctx context.Context, tx *sql.Tx, userID, portfolioName string, referenceDate time.Time) error {
	query := `UPDATE monthly_statements SET status = 'confirmed', updated_at = now()
	WHERE user_id = $1 AND portfolio_name = $2 AND reference_date = $3`
	_, err := tx.ExecContext(ctx, query, userID, portfolioName, referenceDate)
	return err
}
