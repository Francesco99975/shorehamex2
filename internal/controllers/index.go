package controllers

import (
	"log/slog"
	"net/http"

	"github.com/Francesco99975/shorehamex2/internal/auth"
	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/Francesco99975/shorehamex2/internal/database"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/internal/httperr"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/Francesco99975/shorehamex2/views"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

func Index() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("home page", "Index", c.Request().Header.Get("X-Request-ID"))
		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		user, err := auth.GetActiveSession(c.Request(), repo)
		slog.Debug("user", "user", user, "err", err)
		if err != nil && user != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		if user != nil {
			return c.Redirect(http.StatusSeeOther, "/dashboard")
		}

		return c.Redirect(http.StatusSeeOther, "/auth")

	}
}

func Dashboard() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("dashboard", "Dashboard", c.Request().Header.Get("X-Request-ID"))
		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		user, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil && user != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}
		if user == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		data := config.GetDefaultSite(c.Request())
		data.CurrentUser = user

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		userID, err := uuid.Parse(user.ID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Authenticated user ID", slog.String("userID", userID.String()))

		html := helpers.MustRenderHTML(views.Dashboard(data, views.DashboardProps{
			Username: user.Username,
			Email:    user.Email,
		}))

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func Auth() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("auth", "Auth", c.Request().Header.Get("X-Request-ID"))
		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		user, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil && user != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}
		if user != nil {
			return c.Redirect(http.StatusSeeOther, "/dashboard")
		}

		data := config.GetDefaultSite(c.Request())
		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		slog.Debug("Canonical", slog.String("canonical", data.Metatags.Canonical))

		html := helpers.MustRenderHTML(views.Index(data))

		return c.Blob(http.StatusOK, "text/html", html)

	}
}
