package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/Francesco99975/shorehamex2/internal/database"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/internal/repository"

	"github.com/Francesco99975/shorehamex2/internal/controllers"
	"github.com/Francesco99975/shorehamex2/internal/middlewares"
	"github.com/Francesco99975/shorehamex2/views"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func createRouter() *echo.Echo {
	e := echo.New()
	e.Logger.SetOutput(io.Discard)
	e.HideBanner = true
	e.HidePort = true
	e.Use(middlewares.SlogLogger())
	e.Use(middleware.RemoveTrailingSlash())
	e.Use(middlewares.RateLimiter())
	// Apply Gzip middleware, but skip it for /metrics
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/metrics" // Skip compression for /metrics
		},
	}))
	e.Use(middlewares.MonitoringMiddleware())
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()), middlewares.MetricsAccessMiddleware())
	e.GET("/healthcheck", func(c echo.Context) error {
		time.Sleep(5 * time.Second)
		return c.JSON(http.StatusOK, "OK")
	})
	e.POST("/csp-violation-report", func(c echo.Context) error {
		type CSPReport struct {
			DocumentURI        string `json:"document-uri"`
			Referrer           string `json:"referrer"`
			ViolatedDirective  string `json:"violated-directive"`
			EffectiveDirective string `json:"effective-directive"`
			OriginalPolicy     string `json:"original-policy"`
			BlockedURI         string `json:"blocked-uri"`
			StatusCode         int    `json:"status-code"`
			SourceFile         string `json:"source-file"`
			LineNumber         int    `json:"line-number"`
			ColumnNumber       int    `json:"column-number"`
		}

		type CSPPayload struct {
			Report CSPReport `json:"csp-report"`
		}

		var payload CSPPayload
		if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
			slog.Warn("CSP Violation Report (unparsable body)", slog.String("err", err.Error()))
			return c.NoContent(http.StatusOK)
		}

		r := payload.Report
		slog.Warn("CSP Violation",
			slog.String("blocked_uri", r.BlockedURI),
			slog.String("violated_directive", r.ViolatedDirective),
			slog.String("effective_directive", r.EffectiveDirective),
			slog.String("document_uri", r.DocumentURI),
			slog.String("source_file", r.SourceFile),
			slog.Int("line", r.LineNumber),
			slog.Int("col", r.ColumnNumber),
			slog.String("original_policy", r.OriginalPolicy),
		)

		return c.NoContent(http.StatusOK)
	})

	e.GET("/sw.js", func(c echo.Context) error {
		c.Response().Header().Set("Content-Type", "application/javascript")
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.File("./static/sw.js")
	})

	e.Static("/assets", "./static")
	e.GET("/assets/dist/*", func(c echo.Context) error {
		c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return c.File(filepath.Join("./static/dist", c.Param("*")))
	})

	e.GET("/sitemap.xml", func(c echo.Context) error {
		sitemap := config.GetDefaultSite(c.Request()).Sitemap

		if sitemap == nil {
			slog.Warn("Sitemap not configured")
			return c.NoContent(404)
		}

		return c.Blob(200, "application/xml", sitemap)
	})

	e.GET("/robots.txt", func(c echo.Context) error {
		var content string

		baseURL := boot.Environment.URL

		if boot.Environment.GoEnv == enums.Environments.PRODUCTION {
			content = fmt.Sprintf(`User-agent: *
Disallow: /

Sitemap: %s/sitemap.xml
`, baseURL)
		} else {
			content = fmt.Sprintf(`User-agent: *
Disallow: /

Sitemap: %s/sitemap.xml
`, baseURL)
		}

		return c.Blob(200, "text/plain", []byte(content))
	})

	web := e.Group("")

	web.Use(middlewares.SecurityHeaders())

	web.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "form:_csrf,header:X-CSRF-Token",
		CookieName:     "csrf_token",
		CookiePath:     "/",
		CookieHTTPOnly: true,
		CookieSecure:   boot.Environment.GoEnv == enums.Environments.PRODUCTION,
		CookieSameSite: http.SameSiteLaxMode,
		Skipper: func(c echo.Context) bool {
			// Skip CSRF for the /webhook route
			return c.Path() == "/webhook"

		},
	}))

	am := middlewares.NewAuthMiddlewares(repository.New(database.Pool()))

	web.GET("/", controllers.Index())
	web.GET("/auth", controllers.Auth())
	web.POST("auth/2fa/check", controllers.SessionLoginTwoFACheck())
	web.GET("/auth/2fa/reset", controllers.TwoFAResetForm())
	web.POST("/auth/2fa/reset", controllers.TwoFAReset())
	web.POST("auth/2fa/verify", controllers.TwoFAVerifyReset())
	web.DELETE("auth/2fa/cancel", controllers.TwoFACancelReset())
	web.GET("/auth/2fa/restore", controllers.TwoFARestoreForm())
	web.POST("/auth/2fa/restore", controllers.TwoFARestore())
	web.GET("/dashboard", controllers.Dashboard(), am.AuthMiddleware())

	web.GET("/tests", controllers.Tests(), am.AuthMiddleware())
	web.POST("/tests", controllers.UploadTest(), am.AuthMiddleware(), am.IsDeveloperRoleMiddleware())
	web.GET("/tests/options", controllers.TestOptions(), am.AuthMiddleware())

	web.GET("/patients", controllers.Patients(), am.AuthMiddleware())
	web.POST("/patients", controllers.AddPatient(), am.AuthMiddleware())
	web.PUT("/patients/:id", controllers.UpdatePatient(), am.AuthMiddleware())
	web.DELETE("/patients/:id", controllers.DeletePatient(), am.AuthMiddleware())
	web.GET("/patients/search", controllers.SearchPatients(), am.AuthMiddleware())

	web.GET("/assignments", controllers.Assignments(), am.AuthMiddleware())
	web.POST("/assignments", controllers.Assign(), am.AuthMiddleware())

	web.GET("/settings", controllers.Settings(""), am.AuthMiddleware())
	web.GET("/settings/profile", controllers.Settings("profile"), am.AuthMiddleware())
	web.PATCH("/settings/profile/username", controllers.UpdateUsername(), am.AuthMiddleware())
	web.PATCH("/settings/profile/fullname", controllers.UpdateFullname(), am.AuthMiddleware())
	web.PATCH("/settings/profile/email", controllers.UpdateEmail(), am.AuthMiddleware())
	web.GET("/settings/security", controllers.Settings("security"), am.AuthMiddleware())
	web.DELETE("settings/security/session/:id", controllers.RevokeSession(), am.AuthMiddleware())
	web.PATCH("/settings/password", controllers.UpdateUserPassword(), am.AuthMiddleware())
	web.GET("/settings/account", controllers.Settings("account"), am.AuthMiddleware())
	web.POST("/settings/account/activate", controllers.ActivateUser(), am.AuthMiddleware())
	web.DELETE("/settings/account/deactivate", controllers.DeactivateUser(), am.AuthMiddleware())
	web.DELETE("/settings/account/delete", controllers.PermanentlyDeleteUser(), am.AuthMiddleware())
	web.DELETE("/settings/2fa/cancel", controllers.CancelTwoFA(), am.AuthMiddleware())
	web.POST("/settings/2fa/setup", controllers.InitTwoFA(), am.AuthMiddleware())
	web.POST("/settings/2fa/verify", controllers.VerifyTwoFA(), am.AuthMiddleware())
	web.POST("/settings/2fa/complete", controllers.FinalizeTwoFA(), am.AuthMiddleware())
	web.PATCH("/settings/2fa/disable", controllers.DisableTwoFA(), am.AuthMiddleware())
	web.GET("/settings/users", controllers.Settings("users"), am.AuthMiddleware(), am.IsAdminRoleMiddleware())
	web.GET("/settings/users/:id", controllers.GetUser(), am.AuthMiddleware(), am.IsAdminRoleMiddleware())
	web.POST("/settings/users", controllers.CreateUser(), am.AuthMiddleware(), am.IsAdminRoleMiddleware())
	web.PUT("/settings/users/:id", controllers.UpdateUser(), am.AuthMiddleware(), am.IsAdminRoleMiddleware())
	web.PATCH("/settings/users/:id", controllers.ReactivateUserAsAdmin(), am.AuthMiddleware(), am.IsAdminRoleMiddleware())
	web.DELETE("/settings/users/:id", controllers.DeleteUser(), am.AuthMiddleware(), am.IsAdminRoleMiddleware())
	web.GET("/search/users", controllers.SearchUsers(), am.AuthMiddleware(), am.IsAdminRoleMiddleware())
	web.POST("/signup", controllers.SessionSignup())
	web.POST("/verification/manual", controllers.ManualEmailVerification())
	web.POST("/verification/update", controllers.UpdateManualEmailVerification(), am.AuthMiddleware())
	web.GET("/verification/:token", controllers.EmailVerification())
	web.POST("/verification/resend", controllers.ResendEmailVerification())
	web.POST("/login", controllers.SessionLogin())
	web.POST("/logout", controllers.SessionLogout())
	web.GET("/reset", controllers.ResetPage())
	web.GET("/reset/:token", controllers.ResetPageExpress())
	web.POST("/reset/check", controllers.ResetCheck())
	web.POST("/reset/confirm", controllers.ResetUserPassword())
	web.POST("/reset/dev", controllers.ResetDev())
	web.POST("/reset/resend", controllers.ResendReset())

	e.HTTPErrorHandler = serverErrorHandler

	return e
}

func serverErrorHandler(err error, c echo.Context) {
	// Default to internal server error (500)
	code := http.StatusInternalServerError
	var message any = "Internal Server Error"

	// Check if it's an echo.HTTPError
	if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
		message = he.Message
	}

	// Check the Accept header to decide the response format
	if strings.Contains(c.Request().Header.Get("Accept"), "application/json") {
		// Respond with JSON if the client prefers JSON
		_ = c.JSON(code, map[string]any{
			"error":   true,
			"message": message,
			"status":  code,
		})
	} else {
		// Prepare data for rendering the error page (HTML)
		data := config.GetDefaultSite(c.Request())
		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		var title string
		switch code {
		case http.StatusNotFound:
			title = "Not Found"
		case http.StatusUnauthorized:
			title = "Unauthorized"
		case http.StatusForbidden:
			title = "Forbidden"
		case http.StatusInternalServerError:
			title = "Internal Server Error"
		case http.StatusServiceUnavailable:
			title = "Service Unavailable"
		case http.StatusGatewayTimeout:
			title = "Gateway Timeout"
		case http.StatusBadGateway:
			title = "Bad Gateway"
		case http.StatusBadRequest:
			title = "Bad Request"
		case http.StatusConflict:
			title = "Conflict"
		case http.StatusUnprocessableEntity:
			title = "Unprocessable Entity"
		default:
			title = "Internal Server Error"
		}

		var marquee string
		if code >= 500 {
			marquee = "Server Error"
		} else {
			marquee = "Client Error"
		}

		html := helpers.MustRenderHTML(views.Error(data, views.ErrorContentProps{
			Marquee: marquee,
			Code:    fmt.Sprintf("%d", code),
			Title:   title,
			Detail:  message.(string),
		}))

		// Respond with HTML (default) if the client prefers HTML
		_ = c.Blob(code, "text/html; charset=utf-8", html)
	}
}
