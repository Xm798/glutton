package api

import (
	"net/http"

	"github.com/cyrus/glutton/internal/version"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// Deps holds everything handlers need. Each subsystem is wired here in main.
// Fields are pointers/interfaces so tests can pass nil/mocks selectively.
// More fields are added in later tasks (Store, Scheduler, Pool, Bus, etc.).
type Deps struct {
	// Filled in by later tasks.
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
