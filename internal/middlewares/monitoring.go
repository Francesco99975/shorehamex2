package middlewares

import (
	"net"
	"net/http"
	"strings"
	"time"

	"slices"

	"github.com/Francesco99975/shorehamex2/cmd/boot"

	"github.com/Francesco99975/shorehamex2/internal/httperr"
	"github.com/Francesco99975/shorehamex2/internal/monitoring"
	"github.com/labstack/echo/v4"
)

// MonitoringMiddleware tracks request metrics and exposes them for Prometheus
func MonitoringMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			path := c.Path()
			method := c.Request().Method

			// Proceed with the request
			err := next(c)

			// Calculate duration
			duration := time.Since(start).Seconds()
			status := c.Response().Status

			// Sanitize path for metrics (e.g., convert dynamic routes like /user/:id to /user/{id})
			if strings.Contains(path, ":") {
				path = strings.ReplaceAll(path, ":", "{") + "}"
			}

			// Record metrics
			monitoring.IncreaseHTTPRequestCount(method, path, status)
			monitoring.RecordHTTPRequestDuration(method, path, status, duration)

			return err
		}
	}
}

func MetricsAccessMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			herr := httperr.New("metrics access", "MetricsAccessMiddleware", c.Response().Header().Get(echo.HeaderXRequestID))
			auth := c.Request().Header.Get("Authorization")
			if auth == "" {
				return herr.Handle(c.Response(), http.StatusUnauthorized, nil)
			}

			realIP := c.RealIP()
			var ipStr string
			var err error
			if !strings.Contains(realIP, ":") {
				ipStr = realIP
			} else {
				ipStr, _, err = net.SplitHostPort(c.RealIP())
				if err != nil {
					return herr.HandleJSON(c.Response(), http.StatusInternalServerError, err)
				}
			}

			sourceIP := net.ParseIP(ipStr)
			if sourceIP == nil {
				return herr.HandleJSON(c.Response(), http.StatusInternalServerError, err)
			}

			ips, err := net.LookupHost(boot.Environment.Prometheus)
			if err != nil {
				return herr.HandleJSON(c.Response(), http.StatusInternalServerError, err)
			}

			allowed := slices.Contains(ips, sourceIP.String())

			if !allowed {
				return herr.HandleJSON(c.Response(), http.StatusUnauthorized, nil)
			}

			return next(c)
		}
	}
}
