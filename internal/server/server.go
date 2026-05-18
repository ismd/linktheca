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
	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/core/httpx"
	"github.com/ismd/linktheca/internal/library"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	DB     *pgxpool.Pool
	Radar  *RadarDeps // nil when RADAR_ENABLED=false
}

// RadarDeps is wired by the caller (cmd/linktheca-server) when Radar is enabled.
// It owns the River client lifecycle.
type RadarDeps struct {
	Embedder embeddings.Client
	River    *river.Client[pgx.Tx]
	Workers  *river.Workers
}

func New(deps Deps) *http.Server {
	logger := deps.Logger
	cfg := deps.Config

	issuer := coreauth.NewJWTIssuer(cfg.JWTSecret, cfg.JWTAccessTTL)

	// Auth module
	authStore := auth.NewStore(deps.DB)
	authSvc := auth.NewService(authStore, issuer, auth.ServiceConfig{
		RefreshTTL:          cfg.JWTRefreshTTL,
		RegistrationEnabled: cfg.RegistrationEnabled,
	})
	authHTTP := auth.NewHTTP(authSvc, issuer)

	// Library module
	libStore := library.NewStore(deps.DB)
	extractor := content.NewExtractor()
	libSvc := library.NewService(libStore, extractor)
	libHTTP := library.NewHTTP(libSvc)

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

	// Auth — public (rate-limited)
	r.Group(func(r chi.Router) {
		r.Use(httprate.LimitByIP(10, 10*time.Minute))
		r.Post("/auth/register", authHTTP.RegisterHandler())
		r.Post("/auth/login", authHTTP.LoginHandler())
		r.Post("/auth/refresh", authHTTP.RefreshHandler())
	})

	// Auth — protected
	r.Group(func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/auth/logout", authHTTP.LogoutHandler())
		r.Get("/auth/me", authHTTP.MeHandler())
	})

	// Library — all routes require auth
	r.Route("/library", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/", libHTTP.SaveHandler())
		r.Get("/", libHTTP.ListHandler())
		r.Get("/{id}", libHTTP.GetHandler())
		r.Get("/{id}/content", libHTTP.GetDetailHandler())
		r.Patch("/{id}", libHTTP.UpdateHandler())
		r.Delete("/{id}", libHTTP.DeleteHandler())
	})

	if cfg.RadarEnabled && deps.Radar != nil {
		radarStore := radar.NewStore(deps.DB)
		radarSvc := radar.NewService(radarStore, deps.Radar.Embedder)
		radarHTTP := radar.NewHTTP(radarSvc)

		r.Route("/radar", func(r chi.Router) {
			r.Use(coreauth.RequireUser(issuer))

			r.Post("/topics", radarHTTP.CreateTopicHandler())
			r.Get("/topics", radarHTTP.ListTopicsHandler())
			r.Get("/topics/{id}", radarHTTP.GetTopicHandler())
			r.Patch("/topics/{id}", radarHTTP.UpdateTopicHandler())
			r.Delete("/topics/{id}", radarHTTP.DeleteTopicHandler())

			r.Post("/subscriptions", radarHTTP.SubscribeHandler())

			r.Get("/matches", radarHTTP.ListMatchesHandler())
			r.Patch("/matches/{id}", radarHTTP.UpdateMatchHandler())

			r.Get("/status", radarHTTP.StatusHandler())

			r.Group(func(r chi.Router) {
				r.Use(coreauth.RequireAdmin)
				r.Post("/feeds", radarHTTP.AddFeedHandler())
				r.Get("/feeds", radarHTTP.ListFeedsHandler())
			})
		})
	} else {
		r.HandleFunc("/radar", radar.DisabledHandler)
		r.HandleFunc("/radar/*", radar.DisabledHandler)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return srv
}
