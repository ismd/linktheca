package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chicors "github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/ismd/linktheca/internal/auth"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/httpx"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	DB     *pgxpool.Pool
}

func New(deps Deps) *http.Server {
	logger := deps.Logger
	cfg := deps.Config

	issuer := coreauth.NewJWTIssuer(cfg.JWTSecret, cfg.JWTAccessTTL)

	authStore := auth.NewStore(deps.DB)
	authSvc := auth.NewService(authStore, issuer, auth.ServiceConfig{
		RefreshTTL:          cfg.JWTRefreshTTL,
		RegistrationEnabled: cfg.RegistrationEnabled,
	})
	authHTTP := auth.NewHTTP(authSvc, issuer)

	r := chi.NewRouter()

	r.Use(httpx.RequestID)
	r.Use(httpx.RequestLogger(logger))
	r.Use(httpx.Recover(logger))

	if len(cfg.CORSOrigins) > 0 {
		r.Use(chicors.Handler(chicors.Options{
			AllowedOrigins:   cfg.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	r.Group(func(r chi.Router) {
		r.Use(httprate.LimitByIP(10, 10*time.Minute))
		r.Post("/auth/register", authHTTP.RegisterHandler())
		r.Post("/auth/login", authHTTP.LoginHandler())
		r.Post("/auth/refresh", authHTTP.RefreshHandler())
	})

	r.Group(func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/auth/logout", authHTTP.LogoutHandler())
		r.Get("/auth/me", authHTTP.MeHandler())
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return srv
}
