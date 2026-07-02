// Package api exposes the HTTP surface: chi router, auth middleware, handlers.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// ClusterStore is the subset of store queries the cluster handlers need.
type ClusterStore interface {
	CreateCluster(ctx context.Context, arg db.CreateClusterParams) (db.CreateClusterRow, error)
	ListClusters(ctx context.Context) ([]db.ListClustersRow, error)
	GetCluster(ctx context.Context, id uuid.UUID) (db.GetClusterRow, error)
	DeleteCluster(ctx context.Context, id uuid.UUID) (int64, error)
}

// Store is everything the API needs from the database; *db.Queries satisfies it.
type Store interface {
	AuthStore
	ClusterStore
}

type Options struct {
	SecureCookies bool
	// MasterKey encrypts kubeconfigs at rest (32 bytes, SPEC §2.6).
	MasterKey []byte
	// Connector performs cluster connectivity + RBAC checks.
	Connector cluster.Connector
}

type Server struct {
	store         Store
	logger        *slog.Logger
	secureCookies bool
	masterKey     []byte
	connector     cluster.Connector
	limiter       *loginLimiter
}

func NewServer(store Store, logger *slog.Logger, opts Options) *Server {
	return &Server{
		store:         store,
		logger:        logger,
		secureCookies: opts.SecureCookies,
		masterKey:     opts.MasterKey,
		connector:     opts.Connector,
		limiter:       newLoginLimiter(5, 15*time.Minute),
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// Deliberately NOT using middleware.RealIP: it trusts X-Forwarded-For /
	// X-Real-IP, which an attacker could spoof to evade the per-IP login rate
	// limit (GHSA-9g5q-2w5x-hmxf). We use the real TCP peer via r.RemoteAddr.
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

			r.Route("/clusters", func(r chi.Router) {
				r.Get("/", s.handleListClusters)
				r.Post("/", s.handleCreateCluster)
				r.Post("/inspect", s.handleInspectKubeconfig)
				r.Post("/check", s.handleCheckCluster)
				r.Get("/{id}", s.handleGetCluster)
				r.Delete("/{id}", s.handleDeleteCluster)
			})
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
