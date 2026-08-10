package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/financial-aggregator/ledger/internal/auth"
	"github.com/financial-aggregator/ledger/internal/config"
	"github.com/financial-aggregator/ledger/internal/db"
)

type Handler struct {
	store       *db.DB
	repo        *db.Repository
	authService *auth.Service
}

func NewHandler(store *db.DB, cfg *config.Config) *Handler {
	userRepo := db.NewUserRepository(store.Conn)
	repo := db.NewRepository(store.Conn)
	authService := auth.NewService(userRepo, cfg.JwtSigningKey, cfg.JwtIssuer, cfg.JwtExpirationMinutes)
	return &Handler{
		store:       store,
		repo:        repo,
		authService: authService,
	}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", h.Health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", h.Register)
		r.Post("/auth/login", h.Login)

		r.Group(func(r chi.Router) {
			r.Use(h.AuthMiddleware)
			r.Post("/portfolios/{portfolioName}/{referenceDate}/assets", h.PostAssets)
			r.Post("/portfolios/{portfolioName}/{referenceDate}/mov", h.PostMov)
			r.Get("/portfolios/{portfolioName}/{referenceDate}/reconciliation", h.GetReconciliation)
			r.Post("/portfolios/{portfolioName}/{referenceDate}/confirm", h.PostConfirm)
			r.Get("/portfolios", h.ListPortfolios)
			r.Post("/portfolios", h.CreatePortfolio)
			r.Get("/holdings", h.ListHoldings)
			r.Get("/holdings/{holdingID}/transactions", h.ListHoldingTransactions)
		})
	})

	return r
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
