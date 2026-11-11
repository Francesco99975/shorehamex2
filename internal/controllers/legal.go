package controllers

import (
	"net/http"

	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/views"
	"github.com/labstack/echo/v4"
)

func PrivacyPolicy() echo.HandlerFunc {
	return func(c echo.Context) error {
		data := config.GetDefaultSite(c.Request(), c.Path())

		data.CSRF = c.Get("csrf").(string)
		data.Nonce = c.Get("nonce").(string)

		html := helpers.MustRenderHTML(views.PrivacyPolicy(data))

		return c.Blob(http.StatusOK, "text/html; charset=utf-8", html)
	}
}

func Terms() echo.HandlerFunc {
	return func(c echo.Context) error {
		data := config.GetDefaultSite(c.Request(), c.Path())

		data.CSRF = c.Get("csrf").(string)
		data.Nonce = c.Get("nonce").(string)

		html := helpers.MustRenderHTML(views.Terms(data))

		return c.Blob(http.StatusOK, "text/html; charset=utf-8", html)
	}
}
