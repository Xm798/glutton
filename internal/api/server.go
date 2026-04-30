package api

import (
	"net/http"
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
	Store *gorm.DB
	Live  *LiveCounters
	State *scheduler.State
	Burst BurstController
	Bus   *events.Bus
	Loc   *time.Location // used by stats handlers to default `since` to today midnight
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

func New(deps Deps) *Server {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
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

	ch := &configHandlers{store: deps.Store}
	g.GET("/config", ch.get)
	g.PUT("/config", ch.put)

	sh := &sourcesHandlers{store: deps.Store}
	g.GET("/sources", sh.list)
	g.POST("/sources", sh.create)
	g.PUT("/sources/:id", sh.update)
	g.DELETE("/sources/:id", sh.delete)

	stats := &statsHandlers{store: deps.Store, counter: deps.Live, loc: deps.Loc}
	g.GET("/stats/live", stats.live)
	g.GET("/stats/history", stats.history)
	g.GET("/stats/sources", stats.bySource)

	// /api/control gets a tighter rate limit per spec §9 (5 req/s per IP).
	control := g.Group("/control",
		middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(5))))
	ctl := &controlHandlers{state: deps.State, burst: deps.Burst, bus: deps.Bus}
	control.POST("/pause", ctl.pause)
	control.POST("/resume", ctl.resume)
	control.POST("/burst", ctl.burstNow)
	control.POST("/reset-daily", ctl.resetDaily)

	g.GET("/events", (&sseHandlers{bus: deps.Bus}).stream)

	// SPA catch-all — must be last; Echo prioritises more-specific routes above.
	e.GET("/*", spaHandler())

	return &Server{e: e}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.e.ServeHTTP(w, r)
}

func (s *Server) Echo() *echo.Echo { return s.e }
