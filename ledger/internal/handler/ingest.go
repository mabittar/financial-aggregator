package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/financial-aggregator/ledger/internal/auth"
	"github.com/financial-aggregator/ledger/internal/db"
	"github.com/go-chi/chi/v5"
)

type ingestRequest struct {
	IdempotencyKey string          `json:"idempotency_key"`
	ForceUpdate    bool            `json:"force_update"`
	Payload        json.RawMessage `json:"payload"`
}

type ingestResponse struct {
	StatementID string          `json:"statement_id,omitempty"`
	Status      string          `json:"status"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type conflictResponse struct {
	Error    string          `json:"error"`
	Existing json.RawMessage `json:"existing_record"`
}

var errBadRequest = errors.New("invalid request payload")

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "unable to parse request")
		return
	}
	user, err := h.authService.Register(r.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           user.ID,
		"email":        user.Email,
		"display_name": user.DisplayName,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "unable to parse request")
		return
	}

	user, err := h.authService.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.authService.GenerateToken(user, "user")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"access_token": token})
}

func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := auth.ExtractBearerToken(r.Header.Get("Authorization"))
		claims, err := h.authService.ValidateToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication failed")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithUserID(r.Context(), claims.UserID)))
	})
}

func (h *Handler) PostAssets(w http.ResponseWriter, r *http.Request) {
	h.handleIngest(w, r, "assets")
}

func (h *Handler) PostMov(w http.ResponseWriter, r *http.Request) {
	h.handleIngest(w, r, "mov")
}

func (h *Handler) handleIngest(w http.ResponseWriter, r *http.Request, source string) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	portfolioName := chi.URLParam(r, "portfolioName")
	referenceDateParam := chi.URLParam(r, "referenceDate")
	if portfolioName == "" || referenceDateParam == "" {
		writeError(w, http.StatusBadRequest, "portfolio and date are required")
		return
	}

	referenceDate, err := time.Parse("20060102", referenceDateParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "reference date must be YYYYMMDD")
		return
	}

	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "unable to parse request")
		return
	}

	if req.IdempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key is required")
		return
	}
	if len(req.Payload) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "payload is required")
		return
	}

	if err := validateCanonicalPayload(req.Payload); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	normalized := json.RawMessage(req.Payload)
	hashValue := sha256.Sum256(normalized)
	payloadHash := hex.EncodeToString(hashValue[:])

	var response ingestResponse
	err = h.store.WithTransaction(r.Context(), userID, func(ctx context.Context, tx db.Tx) error {
		// Find if idempotency key exists
		existingKey, err := h.repo.FindIdempotencyKey(ctx, tx, req.IdempotencyKey)
		if err != nil {
			return err
		}

		if existingKey != nil && existingKey.UserID == userID {
			if !req.ForceUpdate {
				return &duplicateError{metadata: existingKey.ResponseMetadata}
			}

			// Update existing statement
			statementID, err := h.repo.UpdateMonthlyStatement(ctx, tx, userID, portfolioName, referenceDate, req.IdempotencyKey, normalized, normalized, source)
			if err != nil {
				return err
			}

			metadata := metadataPayload(statementID, "updated")
			if err := h.repo.UpsertIdempotencyKey(ctx, tx, req.IdempotencyKey, userID, payloadHash, metadata); err != nil {
				return err
			}

			response = ingestResponse{StatementID: statementID, Status: "updated", Metadata: metadata}
			return nil
		}

		// Insert new statement
		statementID, err := h.repo.InsertMonthlyStatement(ctx, tx, userID, portfolioName, referenceDate, req.IdempotencyKey, normalized, normalized, source)
		if err != nil {
			return err
		}

		metadata := metadataPayload(statementID, "created")
		if err := h.repo.UpsertIdempotencyKey(ctx, tx, req.IdempotencyKey, userID, payloadHash, metadata); err != nil {
			return err
		}

		response = ingestResponse{StatementID: statementID, Status: "created", Metadata: metadata}
		return nil
	})

	if err != nil {
		var dup *duplicateError
		if errors.As(err, &dup) {
			writeJSON(w, http.StatusConflict, conflictResponse{Error: "duplicate idempotency_key", Existing: dup.metadata})
			return
		}
		writeError(w, http.StatusInternalServerError, "internal ingest failure")
		return
	}

	status := http.StatusCreated
	if req.ForceUpdate {
		status = http.StatusOK
	}
	writeJSON(w, status, response)
}

func (h *Handler) GetReconciliation(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	portfolioName := chi.URLParam(r, "portfolioName")
	referenceDateParam := chi.URLParam(r, "referenceDate")
	if portfolioName == "" || referenceDateParam == "" {
		writeError(w, http.StatusBadRequest, "portfolio and date are required")
		return
	}

	referenceDate, err := time.Parse("20060102", referenceDateParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "reference date must be YYYYMMDD")
		return
	}

	summary, err := h.repo.ListStatementsForPortfolio(r.Context(), userID, portfolioName, referenceDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load reconciliation")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) PostConfirm(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	portfolioName := chi.URLParam(r, "portfolioName")
	referenceDateParam := chi.URLParam(r, "referenceDate")
	if portfolioName == "" || referenceDateParam == "" {
		writeError(w, http.StatusBadRequest, "portfolio and date are required")
		return
	}

	referenceDate, err := time.Parse("20060102", referenceDateParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "reference date must be YYYYMMDD")
		return
	}

	err = h.store.WithTransaction(r.Context(), userID, func(ctx context.Context, tx db.Tx) error {
		return h.repo.ConfirmStatements(ctx, tx, userID, portfolioName, referenceDate)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to confirm portfolio")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

func (h *Handler) ListPortfolios(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	portfolios, err := h.repo.ListPortfolios(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list portfolios")
		return
	}
	writeJSON(w, http.StatusOK, portfolios)
}

func (h *Handler) CreatePortfolio(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "unable to parse request")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	id, err := h.repo.CreatePortfolio(r.Context(), userID, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create portfolio")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "name": req.Name})
}

func (h *Handler) ListHoldings(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	holdings, err := h.repo.ListHoldings(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list holdings")
		return
	}
	writeJSON(w, http.StatusOK, holdings)
}

func (h *Handler) ListHoldingTransactions(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	holdingID := chi.URLParam(r, "holdingID")
	if holdingID == "" {
		writeError(w, http.StatusBadRequest, "holding id is required")
		return
	}

	transactions, err := h.repo.ListHoldingTransactions(r.Context(), userID, holdingID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list transactions")
		return
	}
	writeJSON(w, http.StatusOK, transactions)
}

func validateCanonicalPayload(payload json.RawMessage) error {
	var envelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}
	if len(envelope.Items) == 0 {
		return errors.New("payload must contain an items array with at least one element")
	}
	return nil
}

func metadataPayload(statementID, status string) json.RawMessage {
	payload, _ := json.Marshal(map[string]string{"statement_id": statementID, "status": status})
	return payload
}

type duplicateError struct {
	metadata json.RawMessage
}

func (e *duplicateError) Error() string {
	return "duplicate idempotency_key"
}

func (e *duplicateError) Unwrap() error {
	return errors.New("duplicate idempotency")
}