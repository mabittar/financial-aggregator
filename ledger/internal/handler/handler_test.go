package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/financial-aggregator/ledger/internal/db"
	"github.com/financial-aggregator/ledger/tests"

	"github.com/stretchr/testify/require"
)

func setupTestHandler(t *testing.T) (*Handler, func()) {
	t.Helper()

	_, dbURL, cleanupDB := tests.NewTestDB(t)

	cfg := tests.NewTestConfig(t)
	cfg.DatabaseURL = dbURL

	store, err := db.Connect(context.Background(), cfg)
	require.NoError(t, err)

	h := NewHandler(store, cfg)

	cleanup := func() {
		store.Close()
		cleanupDB()
	}

	return h, cleanup
}

// TestHealthEndpoint_ReturnsOK verifies the health endpoint returns 200 OK
func TestHealthEndpoint_ReturnsOK(t *testing.T) {
	t.Helper()

	h, cleanup := setupTestHandler(t)
	defer cleanup()

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"ok"`)
}

// TestRegisterEndpoint_Success tests successful user registration
func TestRegisterEndpoint_Success(t *testing.T) {
	t.Helper()

	// Setup test environment
	cleanup := tests.SetupTestEnv(t, map[string]string{
		"JWT_SIGNING_KEY":   "test-signing-key-256-bit-minimum-length-required-here-1234",
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpass",
		"POSTGRES_DB":       "testdb",
		"LEDGER_PORT":       "8080",
	})
	defer cleanup()

	// Start test database
	h, cleanup := setupTestHandler(t)
	defer cleanup()
	router := h.Routes()

	// Create registration request
	email := "test-" + strconv.Itoa(rand.Intn(100000)) + "@example.com"
	reqBody := map[string]string{
		"email":        email,
		"password":     "secure_password_123",
		"display_name": "Test User",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Serve HTTP
	router.ServeHTTP(w, req)

	// Assertions
	require.Equal(t, http.StatusCreated, w.Code, "expected created status")

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, email, resp["email"])
	require.Equal(t, "Test User", resp["display_name"])
	require.NotEmpty(t, resp["id"])
}

// TestRegisterEndpoint_InvalidJSON tests malformed JSON request
func TestRegisterEndpoint_InvalidJSON(t *testing.T) {
	t.Helper()

	// Setup test environment
	cleanup := tests.SetupTestEnv(t, map[string]string{
		"JWT_SIGNING_KEY":   "test-signing-key-256-bit-minimum-length-required-here-1234",
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpass",
		"POSTGRES_DB":       "testdb",
		"LEDGER_PORT":       "8080",
	})
	defer cleanup()

	// Start test database
	h, cleanup := setupTestHandler(t)
	defer cleanup()
	router := h.Routes()

	// Create invalid JSON request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Serve HTTP
	router.ServeHTTP(w, req)

	// Assertions
	require.Equal(t, http.StatusBadRequest, w.Code, "expected bad request")
	require.Contains(t, w.Body.String(), `"error":"unable to parse request"`)
}

// TestLoginEndpoint_Success tests successful login
func TestLoginEndpoint_Success(t *testing.T) {
	t.Helper()

	// Setup test environment
	cleanup := tests.SetupTestEnv(t, map[string]string{
		"JWT_SIGNING_KEY":   "test-signing-key-256-bit-minimum-length-required-here-1234",
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpass",
		"POSTGRES_DB":       "testdb",
		"LEDGER_PORT":       "8080",
	})
	defer cleanup()

	// Start test database
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	router := h.Routes()

	// First register a user
	email := "test-" + strconv.Itoa(rand.Intn(100000)) + "@example.com"
	registerReq := map[string]string{
		"email":        email,
		"password":     "secure_password_123",
		"display_name": "Test User",
	}
	registerJSON, _ := json.Marshal(registerReq)
	registerHTTPReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(registerJSON))
	registerHTTPReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()

	router.ServeHTTP(registerResp, registerHTTPReq)
	require.Equal(t, http.StatusCreated, registerResp.Code, "registration should succeed")

	// Now login with the registered user
	loginReq := map[string]string{
		"email":    email,
		"password": "secure_password_123",
	}
	loginJSON, _ := json.Marshal(loginReq)
	loginHTTPReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginJSON))
	loginHTTPReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()

	router.ServeHTTP(loginResp, loginHTTPReq)

	// Assertions
	require.Equal(t, http.StatusOK, loginResp.Code, "expected OK status")
	require.Contains(t, loginResp.Header().Get("Content-Type"), "application/json")

	var resp map[string]string
	require.NoError(t, json.Unmarshal(loginResp.Body.Bytes(), &resp))
	require.NotEmpty(t, resp["access_token"])
	require.NotEmpty(t, resp["access_token"], "access token should not be empty")
}

// TestLoginEndpoint_InvalidCredentials tests login with wrong password
func TestLoginEndpoint_InvalidCredentials(t *testing.T) {
	t.Helper()

	// Setup test environment
	cleanup := tests.SetupTestEnv(t, map[string]string{
		"JWT_SIGNING_KEY":   "test-signing-key-256-bit-minimum-length-required-here-1234",
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpass",
		"POSTGRES_DB":       "testdb",
		"LEDGER_PORT":       "8080",
	})
	defer cleanup()

	// Start test database and create handler
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	router := h.Routes()

	// First register a user
	email := "test-" + strconv.Itoa(rand.Intn(100000)) + "@example.com"
	registerReq := map[string]string{
		"email":        email,
		"password":     "secure_password_123",
		"display_name": "Test User",
	}
	registerJSON, _ := json.Marshal(registerReq)
	registerHTTPReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(registerJSON))
	registerHTTPReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()

	router.ServeHTTP(registerResp, registerHTTPReq)
	require.Equal(t, http.StatusCreated, registerResp.Code, "registration should succeed")

	// Now login with wrong password
	loginReq := map[string]string{
		"email":    email,
		"password": "wrong_password",
	}
	loginJSON, _ := json.Marshal(loginReq)
	loginHTTPReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginJSON))
	loginHTTPReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()

	router.ServeHTTP(loginResp, loginHTTPReq)

	// Assertions
	require.Equal(t, http.StatusUnauthorized, loginResp.Code, "expected unauthorized")
	require.Contains(t, loginResp.Body.String(), `"error":"invalid credentials"`)
}

// TestProtectedRoute_WithoutToken_Returns401 tests accessing protected route without token
func TestProtectedRoute_WithoutToken_Returns401(t *testing.T) {
	t.Helper()

	// Setup test environment
	cleanup := tests.SetupTestEnv(t, map[string]string{
		"JWT_SIGNING_KEY":   "test-signing-key-256-bit-minimum-length-required-here-1234",
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpass",
		"POSTGRES_DB":       "testdb",
		"LEDGER_PORT":       "8080",
	})
	defer cleanup()

	// Start test database and create handler
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	router := h.Routes()

	// Try to access protected route without token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolios", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Assertions
	require.Equal(t, http.StatusUnauthorized, w.Code, "expected unauthorized")
	require.Contains(t, w.Body.String(), `"error":"authentication failed"`)
}

// TestProtectedRoute_WithValidToken_AllowsAccess tests accessing protected route with valid token
func TestProtectedRoute_WithValidToken_AllowsAccess(t *testing.T) {
	t.Helper()

	// Setup test environment
	cleanup := tests.SetupTestEnv(t, map[string]string{
		"JWT_SIGNING_KEY":   "test-signing-key-256-bit-minimum-length-required-here-1234",
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpass",
		"POSTGRES_DB":       "testdb",
		"LEDGER_PORT":       "8080",
	})
	defer cleanup()

	// Start test database and create handler
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	router := h.Routes()

	// First register a user
	email := "test-" + strconv.Itoa(rand.Intn(100000)) + "@example.com"
	registerReq := map[string]string{
		"email":        email,
		"password":     "secure_password_123",
		"display_name": "Test User",
	}
	registerJSON, _ := json.Marshal(registerReq)
	registerHTTPReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(registerJSON))
	registerHTTPReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()

	router.ServeHTTP(registerResp, registerHTTPReq)
	if registerResp.Code != http.StatusOK {
		t.Logf("DEBUG: status=%d, body=%s", registerResp.Code, registerResp.Body.String())
	}
	require.Equal(t, http.StatusCreated, registerResp.Code, "registration should succeed")

	// Login to get token
	loginReq := map[string]string{
		"email":    email,
		"password": "secure_password_123",
	}
	loginJSON, _ := json.Marshal(loginReq)
	loginHTTPReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginJSON))
	loginHTTPReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()

	router.ServeHTTP(loginResp, loginHTTPReq)
	if loginResp.Code != http.StatusOK {
		t.Logf("DEBUG: status=%d, body=%s", loginResp.Code, loginResp.Body.String())
	}
	require.Equal(t, http.StatusOK, loginResp.Code, "login should succeed")

	var loginRespData map[string]string
	require.NoError(t, json.Unmarshal(loginResp.Body.Bytes(), &loginRespData))
	token := loginRespData["access_token"]
	require.NotEmpty(t, token, "token should not be empty")

	// Now access protected route with token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolios", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Logf("DEBUG: status=%d, body=%s", w.Code, w.Body.String())
	}

	// Assertions - should return empty array (no portfolios yet) with 200 OK
	require.Equal(t, http.StatusOK, w.Code, "expected OK status")
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var portfolios []interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &portfolios))
	require.Empty(t, portfolios, "should return empty portfolio list")
}
