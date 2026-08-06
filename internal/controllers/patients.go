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
	"github.com/labstack/echo/v4"
)

func Patients() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("patients", "Patients", c.Request().Header.Get("X-Request-ID"))

		repo := repository.New(database.Pool())

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

		html := helpers.MustRenderHTML(views.Patients(data))

		return c.Blob(http.StatusOK, "text/html", html)

	}
}
