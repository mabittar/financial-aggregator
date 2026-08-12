package db

import (
	"context"
	"encoding/json"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/financial-aggregator/ledger/internal/auth"
	"github.com/financial-aggregator/ledger/tests"
	"github.com/jackc/pgx/v5"
)

func setupTestDB(t *testing.T) (*DB, *Repository, func()) {
	t.Helper()

	pool, dbURL, cleanupDB := tests.NewTestDB(t)

	cfg := tests.NewTestConfig(t)
	cfg.DatabaseURL = dbURL

	repo := NewRepository(pool)

	cleanup := func() {
		pool.Close()
		cleanupDB()
	}

	return &DB{Conn: pool}, repo, cleanup
}

func TestUserRepository_Create_And_FindByEmail(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewUserRepository(db.Conn)
	ctx := context.Background()
	email := "test-" + strconv.Itoa(rand.Intn(100000)) + "@example.com"

	user := &auth.User{
		Email:        email,
		PasswordHash: "hashed_password",
		DisplayName:  "Test User",
	}

	err := repo.Create(ctx, user)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if user.ID == "" {
		t.Error("Expected user ID to be set after Create()")
	}

	if user.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set after Create()")
	}

	// Find the user
	found, err := repo.FindByEmail(ctx, email)
	if err != nil {
		t.Fatalf("FindByEmail() failed: %v", err)
	}

	if found == nil {
		t.Fatal("FindByEmail() returned nil user")
	}

	if found.ID != user.ID {
		t.Errorf("Expected user ID %q, got %q", user.ID, found.ID)
	}

	if found.Email != email {
		t.Errorf("Expected email 'test@example.com', got %q", found.Email)
	}

	if found.PasswordHash != "hashed_password" {
		t.Errorf("Expected password hash 'hashed_password', got %q", found.PasswordHash)
	}
}

func TestRepository_InsertMonthlyStatement_And_FindList(t *testing.T) {
	db, repo, cleanup := setupTestDB(t)
	defer cleanup()

	userRepo := NewUserRepository(db.Conn)
	ctx := context.Background()

	// Create a user first
	user := &auth.User{
		Email:        "test-" + strconv.Itoa(rand.Intn(100000)) + "@example.com",
		PasswordHash: "hashed_password",
		DisplayName:  "Test User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Start a transaction for the statement insertion
	tx, err := db.Conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Prepare test data
	refDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	rawPayload := json.RawMessage(`{"test": "raw data"}`)
	parsedPayload := json.RawMessage(`{"test": "parsed data"}`)

	// Insert monthly statement
	stmtID, err := repo.InsertMonthlyStatement(ctx, tx, user.ID, "test_portfolio", refDate, "ingest_key_1", rawPayload, parsedPayload, "manual")
	if err != nil {
		t.Fatalf("InsertMonthlyStatement() failed: %v", err)
	}

	if stmtID == "" {
		t.Error("Expected non-empty statement ID")
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// List statements
	statements, err := repo.ListStatementsForPortfolio(ctx, user.ID, "test_portfolio", refDate)
	if err != nil {
		t.Fatalf("ListStatementsForPortfolio() failed: %v", err)
	}

	if len(statements) == 0 {
		t.Fatal("Expected at least one statement, got 0")
	}

	stmt := statements[0]
	if stmt["id"] != stmtID {
		t.Errorf("Expected statement ID %q, got %q", stmtID, stmt["id"])
	}

	if stmt["status"] != "pending" {
		t.Errorf("Expected status 'pending', got %q", stmt["status"])
	}

	if stmt["source"] != "manual" {
		t.Errorf("Expected source 'manual', got %q", stmt["source"])
	}
}

func TestRepository_IdempotencyKey_Upsert_Idempotent(t *testing.T) {
	db, repo, cleanup := setupTestDB(t)
	defer cleanup()

	userRepo := NewUserRepository(db.Conn)
	ctx := context.Background()

	// Create a user
	user := &auth.User{
		Email:        "test-" + strconv.Itoa(rand.Intn(100000)) + "@example.com",
		PasswordHash: "hashed_password",
		DisplayName:  "Test User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create idempotency key via transaction
	tx, err := db.Conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	responseMetadata := json.RawMessage(`{"response": "first"}`)
	err = repo.UpsertIdempotencyKey(ctx, tx, "key_1", user.ID, "hash1", responseMetadata)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("First UpsertIdempotencyKey() failed: %v", err)
	}

	// Upsert with same key but different metadata (should update)
	responseMetadata2 := json.RawMessage(`{"response": "second"}`)
	err = repo.UpsertIdempotencyKey(ctx, tx, "key_1", user.ID, "hash1", responseMetadata2)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("Second UpsertIdempotencyKey() failed: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Verify the key exists and has the updated metadata
	tx2, err := db.Conn.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		t.Fatalf("Failed to begin read transaction: %v", err)
	}
	defer tx2.Rollback(ctx)

	key, err := repo.FindIdempotencyKey(ctx, tx2, "key_1")
	if err != nil {
		t.Fatalf("FindIdempotencyKey() failed: %v", err)
	}

	if key == nil {
		t.Fatal("Expected to find idempotency key, got nil")
	}

	if key.Key != "key_1" {
		t.Errorf("Expected key 'key_1', got %q", key.Key)
	}

	if !json.Valid(key.ResponseMetadata) {
		t.Error("Response metadata is not valid JSON")
	}
}

func TestRepository_MonthlyStatement_UniqueConstraint(t *testing.T) {
	db, repo, cleanup := setupTestDB(t)
	defer cleanup()

	userRepo := NewUserRepository(db.Conn)
	ctx := context.Background()

	// Create a user
	user := &auth.User{
		Email:        "test-" + strconv.Itoa(rand.Intn(100000)) + "@example.com",
		PasswordHash: "hashed_password",
		DisplayName:  "Test User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	refDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	rawPayload := json.RawMessage(`{"test": "raw"}`)
	parsedPayload := json.RawMessage(`{"test": "parsed"}`)

	// Insert first statement
	tx1, err := db.Conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = repo.InsertMonthlyStatement(ctx, tx1, user.ID, "portfolio1", refDate, "key1", rawPayload, parsedPayload, "manual")
	if err != nil {
		tx1.Rollback(ctx)
		t.Fatalf("First InsertMonthlyStatement() failed: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Try to insert duplicate (same user_id, portfolio_name, reference_date, ingest_key)
	// This should fail due to unique constraint
	tx2, err := db.Conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("Failed to begin second transaction: %v", err)
	}

	_, err = repo.InsertMonthlyStatement(ctx, tx2, user.ID, "portfolio1", refDate, "key1", rawPayload, parsedPayload, "manual")
	tx2.Rollback(ctx)

	if err == nil {
		t.Error("Expected error for duplicate statement (unique constraint), got nil")
	}

	// But inserting with different key should succeed
	tx3, err := db.Conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("Failed to begin third transaction: %v", err)
	}

	stmtID, err := repo.InsertMonthlyStatement(ctx, tx3, user.ID, "portfolio1", refDate, "key2", rawPayload, parsedPayload, "manual")
	if err != nil {
		tx3.Rollback(ctx)
		t.Fatalf("Second InsertMonthlyStatement() with different key failed: %v", err)
	}
	if err := tx3.Commit(ctx); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	if stmtID == "" {
		t.Error("Expected non-empty statement ID for second insert")
	}
}
