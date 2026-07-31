package middlewares

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Francesco99975/shorehamex2/internal/auth"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/httperr"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

type UserIDKey string

const (
	UserKey UserIDKey = "user_id"
)

type AuthMiddlewares struct {
	repo *repository.Queries
}

func NewAuthMiddlewares(repo *repository.Queries) *AuthMiddlewares {
	return &AuthMiddlewares{repo: repo}
}

func (m *AuthMiddlewares) AuthMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			herr := httperr.New("authorizing user", "AuthMiddleware", c.Request().Header.Get("X-Request-ID"))

			auser, err := auth.GetActiveSession(c.Request(), m.repo)
			if err != nil || auser == nil {
				if auser == nil || errors.Is(err, http.ErrNoCookie) || errors.Is(err, pgx.ErrNoRows) {
					if c.Request().Header.Get("HX-Request") == "true" {
						c.Response().Header().Set("HX-Redirect", "/auth")
						return c.NoContent(http.StatusUnauthorized)
					}
					return c.Redirect(http.StatusSeeOther, "/auth")
				}
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			auth.TouchSession(c.Request(), m.repo, auser.SessionID, auser.LastActivityAt)

			slog.Debug("Authenticated user", slog.String("username", auser.Username))

			c.Request().Header.Set("X-Request-ID", auser.ID)

			ctx := context.WithValue(c.Request().Context(), UserKey, auser.ID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

func (m *AuthMiddlewares) IsDeveloperRoleMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, err := auth.GetActiveSession(c.Request(), m.repo)
			if err != nil || user == nil {
				return c.Redirect(http.StatusSeeOther, "/")
			}
			if user.Role != "DEVELOPER" {
				return c.Redirect(http.StatusSeeOther, "/")
			}
			return next(c)
		}
	}
}

func (m *AuthMiddlewares) IsAdminRoleMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, err := auth.GetActiveSession(c.Request(), m.repo)
			if err != nil || user == nil {
				return c.Redirect(http.StatusSeeOther, "/")
			}
			if user.Role == enums.Roles.USER.String() {
				return c.Redirect(http.StatusSeeOther, "/")
			}
			return next(c)
		}
	}
}
