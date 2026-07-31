package controllers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/Francesco99975/shorehamex2/internal/database"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/internal/httperr"
	"github.com/Francesco99975/shorehamex2/internal/models"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/Francesco99975/shorehamex2/internal/tools"
	"github.com/Francesco99975/shorehamex2/views"
	"github.com/Francesco99975/shorehamex2/views/components"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

func ResetPage() echo.HandlerFunc {
	return func(c echo.Context) error {
		data := config.GetDefaultSite(c.Request())

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		html := helpers.MustRenderHTML(views.PasswordReset(data))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func ResetPageExpress() echo.HandlerFunc {
	return func(c echo.Context) error {
		token := c.Param("token")
		data := config.GetDefaultSite(c.Request())

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		html := helpers.MustRenderHTML(views.PasswordResetExpress(data, token))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func ResetCheck() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("reset_check", "ResetCheck", c.Request().Header.Get("X-Request-ID"))
		email := c.FormValue("email")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		user, err := repo.GetUserByEmail(ctx, email)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, err)
		}

		if user == nil {
			return herr.Handle(c.Response(), http.StatusNotFound, errors.New("user not found during reset password, because user is nil"))
		}

		if !user.IsEmailVerified {
			return herr.Handle(c.Response(), http.StatusForbidden, errors.New("user does not have a verified email"))
		}

		token, err := helpers.GenerateBase62Token(12)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		var passwordReset *repository.CreatePasswordResetRow

		_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("password_resets_pkey"), func(id uuid.UUID) error {
			var insert_err error
			passwordReset, insert_err = repo.CreatePasswordReset(ctx, repository.CreatePasswordResetParams{
				ID:        id,
				UserID:    user.ID,
				Token:     token,
				ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute * 30), Valid: true},
			})
			return insert_err
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, err)
		}

		helpers.ResendPasswordResetTemplate(user.Email, passwordReset.Token)

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.CheckEmailCard(user.Email, csrf))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func ResendReset() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("resending reset", "ResendReset", c.Request().Header.Get("X-Request-ID"))
		email := c.FormValue("email")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		user, err := repo.GetUserByEmail(ctx, email)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, err)
		}

		if user == nil {
			return herr.Handle(c.Response(), http.StatusNotFound, errors.New("user not found during reset password, because user is nil"))
		}

		if !user.IsEmailVerified {
			return herr.Handle(c.Response(), http.StatusForbidden, errors.New("user does not have a verified email"))
		}

		err = repo.DeletePasswordResetByUserID(ctx, user.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("error During old token deletion: %v", err))
		}

		token, err := helpers.GenerateBase62Token(12)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("error During Token generation for password reset: %v", err))
		}

		var passwordReset *repository.CreatePasswordResetRow

		_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("password_resets_pkey"), func(id uuid.UUID) error {
			var insert_err error
			passwordReset, insert_err = repo.CreatePasswordReset(ctx, repository.CreatePasswordResetParams{
				ID:        id,
				UserID:    user.ID,
				Token:     token,
				ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute * 30), Valid: true},
			})
			return insert_err
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("password reset entry was not created: %v", err))
		}

		helpers.ResendPasswordResetTemplate(user.Email, passwordReset.Token)

		tools.SetToastTrigger(c.Response(), enums.InfoToast, "Resent Password Reset Email")
		return c.NoContent(http.StatusAccepted)
	}
}

func ResetUserPassword() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("reset password", "ResetUserPassword", c.Request().Header.Get("X-Request-ID"))
		var payload models.ResetPasswordRequest
		err := c.Bind(&payload)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		err = payload.Validate(1)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data on validate: %v", err))
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction from db: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		passwordReset, err := repo.GetPasswordResetByToken(ctx, payload.Token)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("unable to find password reset token: %v", err))
		}

		if passwordReset.ExpiresAt.Time.Before(time.Now()) {
			return herr.Handle(c.Response(), http.StatusPreconditionFailed, errors.New("password reset token has expired"))
		}

		if passwordReset.Used {
			return herr.Handle(c.Response(), http.StatusConflict, errors.New("password reset token has already been used"))
		}

		user, err := repo.GetUserByID(ctx, passwordReset.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("unable to find user: %v", err))
		}

		if user == nil {
			return herr.Handle(c.Response(), http.StatusNotFound, errors.New("unable to find user since its nil"))
		}

		if !user.IsEmailVerified {
			return herr.Handle(c.Response(), http.StatusPreconditionFailed, errors.New("user email is not verified"))
		}

		hashedPassword, err := helpers.HashPassword(payload.Password)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to hash password: %v", err))
		}

		err = repo.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
			ID:           user.ID,
			PasswordHash: hashedPassword,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to update user password: %v", err))
		}

		err = repo.MarkPasswordResetUsed(ctx, passwordReset.Token)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to mark password reset as used: %v", err))
		}

		err = repo.CleanupExpiredPasswordResets(ctx)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to cleanup expired password resets: %v", err))
		}

		html := helpers.MustRenderHTML(components.ResetSuccessCard())

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func ResetDev() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("reset dev", "ResetDev", c.Request().Header.Get("X-Request-ID"))
		var payload models.ResetPasswordRequest
		err := c.Bind(&payload)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		err = payload.Validate(1)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data on validate: %v", err))
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction from db: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		userUUID, err := uuid.Parse(payload.Token)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to parse uuid: %v", err))
		}

		user, err := repo.GetUserByID(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("unable to find user: %v", err))
		}

		if user == nil {
			return herr.Handle(c.Response(), http.StatusNotFound, errors.New("user not found"))
		}

		hashedPassword, err := helpers.HashPassword(payload.Password)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to hash password: %v", err))
		}

		err = repo.VerifyUserEmail(ctx, user.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to validate email: %v", err))
		}

		err = repo.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
			ID:           user.ID,
			PasswordHash: hashedPassword,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to update user password: %v", err))
		}

		go func() {
			err := database.DeleteCredentialsFile()
			if err != nil {
				slog.Warn("failed to delete credentials file", slog.String("error", err.Error()))
			}
		}()

		c.Response().Header().Set("HX-Redirect", "/auth")
		return c.NoContent(http.StatusOK)
	}
}
