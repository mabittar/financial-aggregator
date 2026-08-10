package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// MockUserRepository is a mock implementation of UserRepository for testing
type MockUserRepository struct {
	users map[string]*User
}

func NewMockUserRepository() *MockUserRepository {
	return &MockUserRepository{
		users: make(map[string]*User),
	}
}

func (m *MockUserRepository) Create(ctx context.Context, user *User) error {
	if _, exists := m.users[user.Email]; exists {
		return errors.New("email already exists")
	}
	user.ID = uuid.NewString()
	user.CreatedAt = time.Now()
	m.users[user.Email] = user
	return nil
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	user, exists := m.users[email]
	if !exists {
		return nil, errors.New("user not found")
	}
	return user, nil
}

// Test constants
const (
	testSigningKey        = "test-signing-key-256-bit-minimum-length-required-here-1234"
	testIssuer            = "test-issuer"
	testExpirationMinutes = 60
	testEmail             = "test@example.com"
	testPassword          = "secure_password_123"
	testDisplayName       = "Test User"
)

func TestRegisterUser_Success(t *testing.T) {
	repo := NewMockUserRepository()
	svc := NewService(repo, testSigningKey, testIssuer, testExpirationMinutes)

	user, err := svc.Register(context.Background(), testEmail, testPassword, testDisplayName)
	if err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	if user == nil {
		t.Fatal("Register() returned nil user")
	}

	if user.Email != testEmail {
		t.Errorf("Expected email %q, got %q", testEmail, user.Email)
	}

	if user.DisplayName != testDisplayName {
		t.Errorf("Expected display name %q, got %q", testDisplayName, user.DisplayName)
	}

	if user.PasswordHash == "" {
		t.Error("Expected non-empty password hash")
	}

	if user.ID == "" {
		t.Error("Expected non-empty user ID")
	}
}

func TestRegisterUser_DuplicateEmail(t *testing.T) {
	repo := NewMockUserRepository()
	svc := NewService(repo, testSigningKey, testIssuer, testExpirationMinutes)

	// Register first user
	_, err := svc.Register(context.Background(), testEmail, testPassword, testDisplayName)
	if err != nil {
		t.Fatalf("First Register() failed: %v", err)
	}

	// Try to register with same email
	_, err = svc.Register(context.Background(), testEmail, "different_password", "Other Name")
	if err == nil {
		t.Error("Expected duplicate email error, got nil")
	}
}

func TestRegisterUser_MissingEmail(t *testing.T) {
	repo := NewMockUserRepository()
	svc := NewService(repo, testSigningKey, testIssuer, testExpirationMinutes)

	_, err := svc.Register(context.Background(), "", testPassword, testDisplayName)
	if err == nil {
		t.Error("Expected error for empty email, got nil")
	}
}

func TestRegisterUser_MissingPassword(t *testing.T) {
	repo := NewMockUserRepository()
	svc := NewService(repo, testSigningKey, testIssuer, testExpirationMinutes)

	_, err := svc.Register(context.Background(), testEmail, "", testDisplayName)
	if err == nil {
		t.Error("Expected error for empty password, got nil")
	}
}

func TestAuthenticateUser_ValidCredentials(t *testing.T) {
	repo := NewMockUserRepository()
	svc := NewService(repo, testSigningKey, testIssuer, testExpirationMinutes)

	// Register user first
	_, err := svc.Register(context.Background(), testEmail, testPassword, testDisplayName)
	if err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	// Authenticate with correct credentials
	user, err := svc.Authenticate(context.Background(), testEmail, testPassword)
	if err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}

	if user == nil {
		t.Fatal("Authenticate() returned nil user")
	}

	if user.Email != testEmail {
		t.Errorf("Expected email %q, got %q", testEmail, user.Email)
	}
}

func TestAuthenticateUser_InvalidPassword(t *testing.T) {
	repo := NewMockUserRepository()
	svc := NewService(repo, testSigningKey, testIssuer, testExpirationMinutes)

	// Register user
	_, err := svc.Register(context.Background(), testEmail, testPassword, testDisplayName)
	if err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	// Try to authenticate with wrong password
	_, err = svc.Authenticate(context.Background(), testEmail, "wrong_password")
	if err == nil {
		t.Error("Expected authentication error with wrong password, got nil")
	}

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticateUser_NonexistentEmail(t *testing.T) {
	repo := NewMockUserRepository()
	svc := NewService(repo, testSigningKey, testIssuer, testExpirationMinutes)

	// Try to authenticate non-existent user
	_, err := svc.Authenticate(context.Background(), "nonexistent@example.com", testPassword)
	if err == nil {
		t.Error("Expected authentication error for non-existent user, got nil")
	}

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}
}

func TestGenerateToken_Success(t *testing.T) {
	user := &User{
		ID:           uuid.NewString(),
		Email:        testEmail,
		PasswordHash: "hash",
		DisplayName:  testDisplayName,
		CreatedAt:    time.Now(),
	}

	svc := NewService(nil, testSigningKey, testIssuer, testExpirationMinutes)
	tokenString, err := svc.GenerateToken(user, "user")
	if err != nil {
		t.Fatalf("GenerateToken() failed: %v", err)
	}

	if tokenString == "" {
		t.Error("GenerateToken() returned empty token string")
	}

	// Verify the token can be parsed
	claims := &Claims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(testSigningKey), nil
	})
	if err != nil {
		t.Fatalf("Failed to parse generated token: %v", err)
	}

	if !token.Valid {
		t.Error("Generated token is not valid")
	}
}

func TestGenerateToken_ClaimsIncluded(t *testing.T) {
	testUserID := uuid.NewString()
	user := &User{
		ID:           testUserID,
		Email:        testEmail,
		PasswordHash: "hash",
		DisplayName:  testDisplayName,
		CreatedAt:    time.Now(),
	}

	svc := NewService(nil, testSigningKey, testIssuer, testExpirationMinutes)
	tokenString, err := svc.GenerateToken(user, "user")
	if err != nil {
		t.Fatalf("GenerateToken() failed: %v", err)
	}

	// Parse and validate claims
	claims := &Claims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	_, err = parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(testSigningKey), nil
	})
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if claims.UserID != testUserID {
		t.Errorf("Expected user_id claim %q, got %q", testUserID, claims.UserID)
	}

	if claims.Email != testEmail {
		t.Errorf("Expected email claim %q, got %q", testEmail, claims.Email)
	}

	if claims.Role != "user" {
		t.Errorf("Expected role claim 'user', got %q", claims.Role)
	}

	if claims.Issuer != testIssuer {
		t.Errorf("Expected issuer %q, got %q", testIssuer, claims.Issuer)
	}
}

func TestGenerateToken_NilUser(t *testing.T) {
	svc := NewService(nil, testSigningKey, testIssuer, testExpirationMinutes)
	_, err := svc.GenerateToken(nil, "user")
	if err == nil {
		t.Error("Expected error for nil user, got nil")
	}
}

func TestValidateToken_ValidToken(t *testing.T) {
	user := &User{
		ID:           uuid.NewString(),
		Email:        testEmail,
		PasswordHash: "hash",
		DisplayName:  testDisplayName,
		CreatedAt:    time.Now(),
	}

	svc := NewService(nil, testSigningKey, testIssuer, testExpirationMinutes)

	// Generate token
	tokenString, err := svc.GenerateToken(user, "user")
	if err != nil {
		t.Fatalf("GenerateToken() failed: %v", err)
	}

	// Validate token
	claims, err := svc.ValidateToken(tokenString)
	if err != nil {
		t.Fatalf("ValidateToken() failed: %v", err)
	}

	if claims == nil {
		t.Fatal("ValidateToken() returned nil claims")
	}

	if claims.UserID != user.ID {
		t.Errorf("Expected user_id %q, got %q", user.ID, claims.UserID)
	}

	if claims.Email != testEmail {
		t.Errorf("Expected email %q, got %q", testEmail, claims.Email)
	}
}

func TestValidateToken_EmptyToken(t *testing.T) {
	svc := NewService(nil, testSigningKey, testIssuer, testExpirationMinutes)
	_, err := svc.ValidateToken("")
	if err == nil {
		t.Error("Expected error for empty token, got nil")
	}
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	svc := NewService(nil, testSigningKey, testIssuer, testExpirationMinutes)

	// Create a token with different signing key
	wrongSvc := NewService(nil, "different-signing-key", testIssuer, testExpirationMinutes)
	user := &User{
		ID:           uuid.NewString(),
		Email:        testEmail,
		PasswordHash: "hash",
		DisplayName:  testDisplayName,
		CreatedAt:    time.Now(),
	}

	tokenString, err := wrongSvc.GenerateToken(user, "user")
	if err != nil {
		t.Fatalf("GenerateToken() with wrong key failed: %v", err)
	}

	// Try to validate with original service (different key)
	_, err = svc.ValidateToken(tokenString)
	if err == nil {
		t.Error("Expected error for invalid signature, got nil")
	}
}

func TestValidateToken_InvalidIssuer(t *testing.T) {
	user := &User{
		ID:           uuid.NewString(),
		Email:        testEmail,
		PasswordHash: "hash",
		DisplayName:  testDisplayName,
		CreatedAt:    time.Now(),
	}

	// Generate token with one issuer
	svc1 := NewService(nil, testSigningKey, "issuer-1", testExpirationMinutes)
	tokenString, err := svc1.GenerateToken(user, "user")
	if err != nil {
		t.Fatalf("GenerateToken() failed: %v", err)
	}

	// Try to validate with different issuer
	svc2 := NewService(nil, testSigningKey, "issuer-2", testExpirationMinutes)
	_, err = svc2.ValidateToken(tokenString)
	if err == nil {
		t.Error("Expected error for invalid issuer, got nil")
	}
}
