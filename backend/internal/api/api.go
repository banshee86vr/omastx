// Package api exposes the HTTP surface: chi router, auth middleware, handlers.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	store         AuthStore
	logger        *slog.Logger
	secureCookies bool
}

func NewServer(store AuthStore, logger *slog.Logger, secureCookies bool) *Server {
	return &Server{store: store, logger: logger, secureCookies: secureCookies}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth, s.requireCSRF)
			r.Get("/auth/me", s.handleMe)
			r.Post("/auth/logout", s.handleLogout)
		})

		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeProblem(w, http.StatusNotFound, "not_found", "Not found",
				"This API route doesn't exist. Check the path and try again.")
		})
	})

	return r
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	s.logger.Error("internal error", "error", err)
	writeProblem(w, http.StatusInternalServerError, "internal", "Something went wrong",
		"An unexpected error occurred on the server. Try again; if it persists, check the backend logs.")
}
