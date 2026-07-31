package middlewares

import (
	"log/slog"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

var skipPaths = map[string]bool{
	"/health":      true,
	"/ping":        true,
	"/favicon.ico": true,
	"/robots.txt":  true,
}

var skipPrefixes = []string{
	"/static/",
	"/assets/",
	"/public/",
}

func shouldSkip(path string) bool {
	if skipPaths[path] {
		return true
	}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func SlogLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			path := c.Request().URL.Path

			err := next(c)
			if err != nil {
				c.Error(err)
			}

			// always skip these paths
			if shouldSkip(path) {
				return nil
			}

			status := c.Response().Status

			fields := []any{
				slog.String("component", "http"),
				slog.String("method", c.Request().Method),
				slog.String("path", c.Request().URL.Path),
				slog.Int("status", status),
				slog.Duration("latency", time.Since(start)),
				slog.String("host", c.Request().Host),
				slog.String("user_agent", c.Request().UserAgent()),
				slog.Int("bytes_in", int(c.Request().ContentLength)),
				slog.Int("bytes_out", int(c.Response().Size)),
				slog.String("remote_ip", c.Request().RemoteAddr),
				slog.String("request_id", c.Request().Header.Get("X-Request-ID")),
			}

			switch {
			case status >= 500:
				slog.Error("request", fields...)
			case status >= 400:
				slog.Warn("request", fields...)
			default:
				slog.Info("request", fields...)
			}

			return nil
		}
	}
}
