package controllers

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/auth"
	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/Francesco99975/shorehamex2/internal/database"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/internal/httperr"
	"github.com/Francesco99975/shorehamex2/internal/models"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/Francesco99975/shorehamex2/views/components"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
)

func CancelTwoFA() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("cancel twofa", "CancelTwoFA", c.Request().Header.Get("X-Request-ID"))
		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		auser, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		if auser == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		userUUID, err := uuid.Parse(auser.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		user, err := repo.GetUserByID(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("user not found: %v", err))
		}

		csrf := c.Get("csrf").(string)
		html := helpers.MustRenderHTML(components.TwoFactorCard(user.TwofaEnabled, csrf))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func InitTwoFA() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("init twofa", "InitTwoFA", c.Request().Header.Get("X-Request-ID"))

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		auser, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		if auser == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		userUUID, err := uuid.Parse(auser.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		user, err := repo.GetUserByID(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("user not found: %v", err))
		}

		key, err := totp.Generate(totp.GenerateOpts{
			Issuer:      config.GetDefaultSite(c.Request()).AppName,
			AccountName: user.Email,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not generate code: %v", err))
		}

		encryptionKey, err := helpers.ParseBase64Key(boot.Environment.TwofaKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not parse twofa key: %v", err))
		}

		encryptedTwofaSecret, err := helpers.Encrypt([]byte(key.Secret()), encryptionKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not encrypt twofa secret: %v", err))
		}

		_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("pending_auth_challenges_pkey"), func(u uuid.UUID) error {
			_, err := repo.CreatePendingAuthChallenge(ctx, repository.CreatePendingAuthChallengeParams{
				ID:     u,
				UserID: userUUID,
				Mode:   enums.TwofaModes.REGISTRATION.String(),
				Secret: &encryptedTwofaSecret,
				ExpiresAt: pgtype.Timestamptz{
					Time:  time.Now().Add(time.Duration(time.Minute * 10)),
					Valid: true,
				},
			})
			return err
		})

		image, err := key.Image(200, 200)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not generate qr code: %v", err))
		}

		var buf bytes.Buffer
		if err := png.Encode(&buf, image); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not generate qr code: %v", err))
		}

		pngBytes := buf.Bytes()
		base64Str := base64.StdEncoding.EncodeToString(pngBytes)
		qr_code := fmt.Sprintf("data:image/png;base64,%s", base64Str)

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.TwoFAQRCodeCard(components.TwoFASetupProps{
			QRCodeDataURL: qr_code,
			Secret:        key.Secret(),
			CSRF:          csrf,
		}))

		return c.Blob(http.StatusAccepted, "text/html", html)

	}
}

func VerifyTwoFA() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("verifying twofa", "VerifyTwoFA", c.Request().Header.Get("X-Request-ID"))

		otp := c.FormValue("otp")

		if otp == "" {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data"))
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		auser, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		if auser == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		userUUID, err := uuid.Parse(auser.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not parse user id: %v", err))
		}

		challenge, err := repo.GetPendingAuthChallengeByUser(ctx, repository.GetPendingAuthChallengeByUserParams{
			UserID: userUUID,
			Mode:   enums.TwofaModes.REGISTRATION.String(),
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not get pending auth challenge: %v", err))
		}

		encrypted_totp_secret := challenge.Secret

		encryptionKey, err := helpers.ParseBase64Key(boot.Environment.TwofaKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not parse twofa key: %v", err))
		}

		totp_secret, err := helpers.Decrypt(*encrypted_totp_secret, encryptionKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not decrypt twofa secret: %v", err))
		}

		if !totp.Validate(otp, string(totp_secret)) {
			return herr.Handle(c.Response(), http.StatusUnauthorized, errors.New("totp validation failed"))
		}

		err = repo.EnableUser2FA(ctx, repository.EnableUser2FAParams{
			TwofaSecret: encrypted_totp_secret,
			ID:          userUUID,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not enable 2fa: %v", err))
		}

		var recovery_backup_codes *helpers.BackupCodes

		_, err = helpers.GenerateProofBulkUUIDV4(10, database.IsPKCollision("twofa_backup_codes_pkey"), func(ids []uuid.UUID) error {
			var gen_err error
			recovery_backup_codes, gen_err = helpers.GenerateBackupCodes(ids)
			if gen_err != nil {
				return gen_err
			}

			err = repo.CreateBackupCodes(ctx, repository.CreateBackupCodesParams{
				Column1: recovery_backup_codes.IDs,
				Column2: slices.Repeat([]uuid.UUID{userUUID}, len(recovery_backup_codes.IDs)),
				Column3: recovery_backup_codes.Hashed,
			})

			return err
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not generate backup codes: %v", err))
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.TwoFARecoverySecretCard(strings.Join(recovery_backup_codes.Plain, ","), csrf))

		return c.Blob(http.StatusAccepted, "text/html", html)
	}
}

func DisableTwoFA() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("disabling twofa", "DisableTwoFA", c.Request().Header.Get("X-Request-ID"))

		var payload models.DisableTwoFARequest

		if err := c.Bind(&payload); err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		err := payload.Validate()
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		auser, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to get active session: %v", err))
		}
		if auser == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		userUUID, err := uuid.Parse(auser.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		secrets, err := repo.GetUser2FASecret(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("user not found: %v", err))
		}

		if !helpers.CheckPasswordHash(payload.Password, secrets.PasswordHash) {
			return herr.Handle(c.Response(), http.StatusUnauthorized, errors.New("invalid password"))
		}

		encryptionKey, err := helpers.ParseBase64Key(boot.Environment.TwofaKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not parse twofa key: %v", err))
		}

		totp_secret, err := helpers.Decrypt(*secrets.TwofaSecret, encryptionKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not decrypt twofa secret: %v", err))
		}

		if !totp.Validate(payload.Otp, string(totp_secret)) {
			return herr.Handle(c.Response(), http.StatusUnauthorized, errors.New("totp validation failed"))
		}

		err = repo.DisableUser2FA(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not disable 2fa: %v", err))
		}

		err = repo.DeleteUserBackupCodes(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not delete 2fa backup codes: %v", err))
		}

		// Send Email to user notifying them of the 2FA being disabled

		csrf := c.Get("csrf").(string)
		html := helpers.MustRenderHTML(components.TwoFactorCard(false, csrf))

		return c.Blob(http.StatusAccepted, "text/html", html)

	}
}

func FinalizeTwoFA() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("finalizing twofa", "FinalizeTwoFA", c.Request().Header.Get("X-Request-ID"))

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		auser, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		if auser == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		userUUID, err := uuid.Parse(auser.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		user, err := repo.GetUserByID(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("user not found: %v", err))
		}

		csrf := c.Get("csrf").(string)
		html := helpers.MustRenderHTML(components.TwoFactorCard(user.TwofaEnabled, csrf))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}
