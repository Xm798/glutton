package api

import "github.com/labstack/echo/v4"

// AuthMiddleware is a no-op in v1. Stubbed so v2 can wire real auth without
// reshaping handlers or routes.
func AuthMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return next(c) }
	}
}
