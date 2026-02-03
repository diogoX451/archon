package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/diogoX451/archon/internal/api/dto"
	"github.com/diogoX451/archon/internal/core/ports"
	"github.com/diogoX451/archon/internal/store"
)

// Server encapsula todas dependências da API
type Server struct {
	router     *chi.Mux
	eventBus   ports.EventBus
	ruleStore  RuleStore
	stateStore store.StateStore
}

// NewServer cria server com dependências injetadas
func NewServer(eventBus ports.EventBus, stateStore store.StateStore, ruleStore RuleStore) *Server {
	s := &Server{
		router:     chi.NewRouter(),
		eventBus:   eventBus,
		ruleStore:  ruleStore,
		stateStore: stateStore,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(30 * time.Second))
	s.router.Use(jsonContentType)
}

func (s *Server) setupRoutes() {
	// Health
	s.router.Get("/health", s.handleHealth)

	// API v1
	s.router.Route("/api/v1", func(r chi.Router) {
		// Workflows
		r.Post("/workflows", s.handleCreateWorkflow)
		r.Get("/workflows/{id}", s.handleGetWorkflow)
		r.Get("/workflows/{id}/status", s.handleGetStatus)
		r.Get("/workflows/{id}/result", s.handleGetResult)
		r.Post("/workflows/{id}/agents", s.handleAddAgent)
		r.Post("/workflows/{id}/connections", s.handleConnect)

		r.Post("/plan", s.handlePlan)

		r.Post("/rules", s.handleDefineRule)
		r.Get("/rules", s.handleListRules)
		r.Get("/rules/{a}/{b}", s.handleGetRule)

		r.Post("/webhooks/needs/{correlation_id}", s.handleNeedResponse)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Handler: Health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, dto.HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "0.1.0",
	})
}

// Helper: JSON content-type
func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// Helper: Responder JSON
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// Helper: Responder erro
func respondError(w http.ResponseWriter, status int, code, message string) {
	respondJSON(w, status, dto.ErrorResponse{
		Error: message,
		Code:  code,
	})
}
