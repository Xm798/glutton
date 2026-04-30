package api

import (
	"net/http"

	"github.com/cyrus/glutton/internal/version"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// Deps holds everything handlers need. Each subsystem is wired here in main.
// Fields are pointers/interfaces so tests can pass nil/mocks selectively.
// More fields are added in later tasks (Scheduler, Pool, Bus, etc.).
type Deps struct {
	Store *gorm.DB
	Live  *LiveCounters
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

	stats := &statsHandlers{store: deps.Store, counter: deps.Live}
	g.GET("/stats/live", stats.live)
	g.GET("/stats/history", stats.history)
	g.GET("/stats/sources", stats.bySource)

	// /api/control gets a tighter rate limit per spec §9 (5 req/s per IP).
	// Routes are wired in Task 17; here we only set up the rate-limited group.
	control := g.Group("/control",
		middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(5))))
	_ = control // populated in Task 17

	return &Server{e: e}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.e.ServeHTTP(w, r)
}

func (s *Server) Echo() *echo.Echo { return s.e }
