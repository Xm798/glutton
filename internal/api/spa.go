package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

//go:embed all:spa_dist
var spaDist embed.FS

// spaHandler serves the embedded SPA, falling back to index.html for client-side routes.
func spaHandler() echo.HandlerFunc {
	sub, err := fs.Sub(spaDist, "spa_dist")
	if err != nil {
		// Embed missing or corrupt; serve a clear stub.
		return func(c echo.Context) error {
			return c.String(http.StatusNotImplemented,
				"web UI not embedded. Run `make web` then rebuild.")
		}
	}
	fileServer := http.FileServer(http.FS(sub))
	return func(c echo.Context) error {
		req := c.Request()
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path != "" {
			if f, err := sub.Open(path); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(c.Response(), req)
				return nil
			}
		}
		// SPA fallback: serve index.html so the React router takes over.
		req2 := req.Clone(req.Context())
		req2.URL.Path = "/"
		fileServer.ServeHTTP(c.Response(), req2)
		return nil
	}
}
