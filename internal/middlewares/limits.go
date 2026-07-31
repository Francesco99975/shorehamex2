package middlewares

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/google/uuid"

	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/httperr"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// rateLimiter returns a configured middleware.RateLimiter
func RateLimiter() echo.MiddlewareFunc {
	herr := httperr.New("rate limiter", "RateLimiter", fmt.Sprintf("rate_limiter-middleware_%s", uuid.New().String()))
	// Config per environment
	var config middleware.RateLimiterConfig

	if boot.Environment.GoEnv == enums.Environments.DEVELOPMENT {
		// DEV: Generous limits (for testing)
		config = middleware.RateLimiterConfig{
			Skipper: middleware.DefaultSkipper,
			Store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
				Rate:      100, // 100 req/sec
				Burst:     200,
				ExpiresIn: 1 * time.Minute,
			}),
			IdentifierExtractor: func(c echo.Context) (string, error) {
				return c.RealIP(), nil
			},
			ErrorHandler: func(c echo.Context, err error) error {
				if strings.Contains(c.Request().Header.Get("Accept"), "application/json") {
					return c.JSON(http.StatusTooManyRequests, map[string]string{
						"error": "Too many requests (dev mode)",
					})
				} else {
					return herr.HandleEchoPage(http.StatusTooManyRequests, err)
				}
			},
			DenyHandler: func(c echo.Context, identifier string, err error) error {
				c.Response().Header().Set("Retry-After", "60")
				if strings.Contains(c.Request().Header.Get("Accept"), "application/json") {
					return c.JSON(http.StatusTooManyRequests, map[string]string{
						"error": "Rate limit exceeded. Try again later.",
					})
				} else {
					return herr.HandleEchoPage(http.StatusTooManyRequests, err)
				}
			},
		}
	} else {
		// PROD: Strict, secure limits
		config = middleware.RateLimiterConfig{
			Skipper: middleware.DefaultSkipper,
			Store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
				Rate:      50, // 10 req/sec per IP
				Burst:     80,
				ExpiresIn: 3 * time.Minute,
			}),
			IdentifierExtractor: func(c echo.Context) (string, error) {
				// Use RealIP (respects X-Forwarded-For in prod)
				return c.RealIP(), nil
			},
			ErrorHandler: func(c echo.Context, err error) error {
				if strings.Contains(c.Request().Header.Get("Accept"), "application/json") {
					return c.JSON(http.StatusTooManyRequests, map[string]string{
						"error": "Too many requests",
					})
				} else {
					return herr.Handle(c.Response(), http.StatusTooManyRequests, err)
				}

			},
			DenyHandler: func(c echo.Context, identifier string, err error) error {
				c.Response().Header().Set("Retry-After", "180")
				if strings.Contains(c.Request().Header.Get("Accept"), "application/json") {
					return c.JSON(http.StatusTooManyRequests, map[string]string{
						"error":   "Rate limit exceeded",
						"retryIn": "180",
					})
				} else {
					return herr.HandleEchoPage(http.StatusTooManyRequests, err)
				}
			},
		}
	}

	return middleware.RateLimiterWithConfig(config)
}
