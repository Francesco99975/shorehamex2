package middlewares

import (
	"fmt"
	"strings"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/labstack/echo/v4"
)

func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// ---- 1. Generate a fresh nonce for this request ----
			nonce, err := helpers.GenerateNonce()
			if err != nil {
				return err
			}
			c.Set("nonce", nonce) // <-- used in templ: {{ .nonce }}

			// ---- 2. Environment flag (only for HSTS / X-Frame-Options) ----
			isDev := boot.Environment.GoEnv == enums.Environments.DEVELOPMENT

			// ---- 3. Core CSP directives (identical for dev & prod) ----
			csp := []string{
				"default-src 'self'",
				// Alpine & HTMX are loaded from a nonced script → strict-dynamic
				fmt.Sprintf("script-src 'nonce-%s' 'strict-dynamic' 'unsafe-eval'", nonce),
				// HTMX fetch / WebSocket
				fmt.Sprintf("connect-src 'self' wss://%s ws://%s",
					boot.Environment.Host, boot.Environment.Host),
				// Tailwind + Alpine inline styles
				"style-src 'self' 'unsafe-inline'",
				"img-src 'self' data: blob:",
				"font-src 'self' data:",
				"media-src 'self'",
				"frame-src 'none'",
				"object-src 'none'",
				"frame-ancestors 'none'",
				"base-uri 'self'",
				"report-uri /csp-violation-report",
			}

			// ---- 4. Build the final header ----
			cspHeader := joinDirectives(csp)

			// ---- 5. Set all security headers ----
			c.Response().Header().Set("Content-Security-Policy", cspHeader)
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			c.Response().Header().Set("X-XSS-Protection", "0") // deprecated
			c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// HSTS only in production
			if !isDev {
				c.Response().Header().Set(
					"Strict-Transport-Security",
					"max-age=31536000; includeSubDomains; preload",
				)
			}

			// X-Frame-Options
			if isDev {
				c.Response().Header().Set("X-Frame-Options", "SAMEORIGIN")
			}

			// Permissions-Policy
			c.Response().Header().Set(
				"Permissions-Policy",
				"geolocation=(), microphone=(), camera=(), payment=(), fullscreen=(self)",
			)

			return next(c)
		}
	}
}

// Helper to join CSP directives safely
func joinDirectives(dirs []string) string {
	clean := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d = strings.TrimSpace(d); d != "" {
			clean = append(clean, d)
		}
	}
	return strings.Join(clean, "; ")
}
