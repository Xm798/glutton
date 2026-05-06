package api

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cyrus/glutton/internal/events"
	"github.com/cyrus/glutton/internal/scheduler"
	"github.com/cyrus/glutton/internal/version"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// Deps holds everything handlers need. Each subsystem is wired here in main.
// Fields are pointers/interfaces so tests can pass nil/mocks selectively.
type Deps struct {
	Store           *gorm.DB
	Live            *LiveCounters
	State           *scheduler.State
	Burst           BurstController
	Bus             *events.Bus
	Loc             *time.Location // used by stats handlers to default `since` to today midnight
	SourcesReloader SourcesReloader
}

// BurstController allows /api/control/burst to grant a temporary window
// override, capped by elapsed minutes and/or downloaded bytes (whichever
// hits first). Implemented in main; stub-friendly for tests.
type BurstController interface {
	Burst(minutes int, bytes int64)
}

type Server struct {
	e *echo.Echo
}

// corsEnvVar lets operators opt into CORS (default = closed). Comma-separated
// list of origins, or "*" for any. Empty = the Echo CORS middleware is not
// installed, which matches the "no CORS" default.
const corsEnvVar = "GLUTTON_CORS_ORIGINS"

// bodyLimit caps any incoming request body. 1 MiB is well above what any of
// our handlers needs (config PUT is ~1 KB; sources POST is a few hundred B).
const bodyLimit = "1M"

func New(deps Deps) *Server {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	// Body size cap (M-2 hardening): protect downstream handlers from
	// pathological input. SSE stream and metrics are GETs with no body.
	e.Use(middleware.BodyLimit(bodyLimit))

	// Security headers (M-2 hardening). Echo's Secure() middleware sets
	// X-Content-Type-Options=nosniff, X-Frame-Options=SAMEORIGIN,
	// X-XSS-Protection legacy, and a basic CSP. We tune CSP to allow our
	// own SPA assets (same-origin) plus inline styles (Tailwind generates
	// a small <style> block) while keeping default-src self.
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "SAMEORIGIN",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: true,
		ContentSecurityPolicy: "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; connect-src 'self'",
		ReferrerPolicy:        "no-referrer",
	}))

	// CORS: closed by default. Operators that need cross-origin (e.g.
	// running the SPA from a different host) opt in via GLUTTON_CORS_ORIGINS.
	if origins := strings.TrimSpace(os.Getenv(corsEnvVar)); origins != "" {
		var allow []string
		for _, o := range strings.Split(origins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allow = append(allow, o)
			}
		}
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins:     allow,
			AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
			AllowHeaders:     []string{"Authorization", "Content-Type"},
			AllowCredentials: false,
			MaxAge:           600,
		}))
	}

	e.Use(AuthMiddleware())

	e.GET("/metrics", echo.WrapHandler(metricsHandler))

	g := e.Group("/api")
	g.GET("/version", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"version": version.Version,
			"commit":  version.Commit,
			"date":    version.Date,
		})
	})

	ch := &configHandlers{store: deps.Store, bus: deps.Bus}
	g.GET("/config", ch.get)
	g.PUT("/config", ch.put)

	sh := &sourcesHandlers{store: deps.Store, bus: deps.Bus, reloader: deps.SourcesReloader}
	g.GET("/sources", sh.list)
	g.POST("/sources", sh.create)
	g.PUT("/sources/:id", sh.update)
	g.DELETE("/sources/:id", sh.delete)

	stats := &statsHandlers{store: deps.Store, counter: deps.Live, loc: deps.Loc}
	g.GET("/stats/live", stats.live)
	g.GET("/stats/history", stats.history)
	g.GET("/stats/sources", stats.bySource)

	eh := &eventsHandlers{store: deps.Store}
	g.GET("/events/history", eh.list)
	g.GET("/events/types", EventTypesHandler())

	// /api/control gets a tighter rate limit per spec §9 (5 req/s per IP).
	control := g.Group("/control",
		middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(5))))
	ctl := &controlHandlers{state: deps.State, burst: deps.Burst, bus: deps.Bus}
	control.POST("/pause", ctl.pause)
	control.POST("/resume", ctl.resume)
	control.POST("/burst", ctl.burstNow)
	control.POST("/reset-daily", ctl.resetDaily)
	control.GET("/status", ctl.status)

	g.GET("/events", (&sseHandlers{bus: deps.Bus}).stream)

	// SPA catch-all — must be last; Echo prioritises more-specific routes above.
	e.GET("/*", spaHandler())

	return &Server{e: e}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.e.ServeHTTP(w, r)
}

func (s *Server) Echo() *echo.Echo { return s.e }
