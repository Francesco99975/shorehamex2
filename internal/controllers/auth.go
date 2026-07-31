package controllers

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"log/slog"
	"net/http"
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
	"github.com/Francesco99975/shorehamex2/internal/tools"
	"github.com/Francesco99975/shorehamex2/views"
	"github.com/Francesco99975/shorehamex2/views/components"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/blake2b"
)

func SessionSignup() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("signup", "SessionSignup", c.Request().Header.Get("X-Request-ID"))

		var payload models.SignupRequest
		err := c.Bind(&payload)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}
		slog.Debug("Signup payload", slog.Any("payload", payload))

		err = payload.ValidateAndNormalize(1)
		if err != nil {
			return herr.Why(err.Error()).Handle(c.Response(), http.StatusBadRequest, err)
		}
		slog.Debug("Normalized signup payload", slog.Any("payload", payload))

		ctx := c.Request().Context()

		hashedPassword, err := helpers.HashPassword(payload.Password)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		slog.Debug("Hashed password", slog.String("hashedPassword", hashedPassword))

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		existsEmail, err := repo.ExistsUserWithEmail(ctx, payload.Email)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		if existsEmail {
			return herr.Handle(c.Response(), http.StatusConflict, errors.New("email exists"))
		}

		existsUsername, err := repo.ExistsUserWithUsername(ctx, payload.Username)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		if existsUsername {
			return herr.Handle(c.Response(), http.StatusConflict, errors.New("username exists"))
		}

		var newUser *repository.CreateUserRow

		_, err = helpers.GenerateProofUUIDV7(database.IsPKCollision("users_pkey"), func(id uuid.UUID) error {
			var insert_err error
			newUser, insert_err = repo.CreateUser(ctx, repository.CreateUserParams{ID: id, Role: enums.Roles.USER.String(), Username: payload.Username, Email: payload.Email, PasswordHash: hashedPassword})
			return insert_err
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		slog.Debug("New user", slog.Any("newUser", newUser))

		token, err := helpers.GenerateBase62Token(8)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		slog.Debug("Generated token", slog.String("token", token))

		var ev *repository.CreateEmailVerificationRow

		_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("email_verifications_pkey"), func(id uuid.UUID) error {
			var insert_err error
			ev, insert_err = repo.CreateEmailVerification(ctx, repository.CreateEmailVerificationParams{ID: uuid.New(), UserID: newUser.ID, Token: token, Email: newUser.Email, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Duration(30 * time.Minute)), Valid: true}})
			return insert_err
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		helpers.ResendEmailVerificationTemplate(newUser.Email, ev.Token)

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.EmailVerification(payload.Email, csrf, "/verification/manual"))

		return c.Blob(http.StatusCreated, "text/html", html)
	}
}

func ResendEmailVerification() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("resending email verification", "ResendEmailVerification", c.Request().Header.Get("X-Request-ID"))
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

		if user.IsEmailVerified {
			return herr.Handle(c.Response(), http.StatusConflict, errors.New("user is already verified"))
		}

		err = repo.DeleteEmailVerificationByUserID(ctx, user.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		token, err := helpers.GenerateBase62Token(8)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		slog.Debug("Generated token", slog.String("token", token))

		var ev *repository.CreateEmailVerificationRow

		_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("email_verifications_pkey"), func(id uuid.UUID) error {
			var insert_err error
			ev, insert_err = repo.CreateEmailVerification(ctx, repository.CreateEmailVerificationParams{ID: uuid.New(), UserID: user.ID, Token: token, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Duration(30 * time.Minute)), Valid: true}})
			return insert_err
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		helpers.ResendEmailVerificationTemplate(user.Email, ev.Token)

		tools.SetToastTrigger(c.Response(), enums.InfoToast, "Resent email verification")
		return c.NoContent(http.StatusAccepted)

	}
}

func EmailVerification() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("email verification", "EmailVerification", c.Request().Header.Get("X-Request-ID"))
		payload := models.VerifyEmailRequest{Token: c.Param("token")}

		err := payload.Validate()
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		verification, err := repo.GetEmailVerificationByToken(ctx, payload.Token)

		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		if verification.ExpiresAt.Time.Before(time.Now()) {
			return herr.Handle(c.Response(), http.StatusBadRequest, errors.New("token expired"))
		}

		err = repo.VerifyUserEmail(ctx, verification.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		err = repo.MarkEmailVerificationUsed(ctx, verification.Token)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		return c.Redirect(http.StatusSeeOther, "/")
	}
}

func ManualEmailVerification() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("manual email verification", "ManualEmailVerification", c.Request().Header.Get("X-Request-ID"))
		var payload models.VerifyEmailRequest
		err := c.Bind(&payload)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		err = payload.Validate()
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		verification, err := repo.GetEmailVerificationByToken(ctx, payload.Token)

		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		if verification.ExpiresAt.Time.Before(time.Now()) {
			return herr.Handle(c.Response(), http.StatusBadRequest, errors.New("token expired"))
		}

		err = repo.VerifyUserEmail(ctx, verification.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		err = repo.MarkEmailVerificationUsed(ctx, verification.Token)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		c.Response().Header().Set("HX-Redirect", "/auth")
		return c.NoContent(http.StatusOK)
	}
}

func SessionLogin() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("login", "SessionLogin", c.Request().Header.Get("X-Request-ID"))
		var payload models.LoginRequest
		err := c.Bind(&payload)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		slog.Debug("Login payload", slog.Any("payload", payload))

		err = payload.Validate()
		if err != nil {
			return herr.Why(err.Error()).Handle(c.Response(), http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		user, err := repo.GetUserByEmailOrUsername(ctx, payload.EmailOrUsername)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, err)
		}

		if !user.IsEmailVerified && user.Role != string(enums.Roles.DEVELOPER) {
			token, err := helpers.GenerateBase62Token(8)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}
			slog.Debug("Generated token", slog.String("token", token))

			var ev *repository.CreateEmailVerificationRow

			_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("email_verifications_pkey"), func(id uuid.UUID) error {
				var insert_err error
				ev, insert_err = repo.CreateEmailVerification(ctx, repository.CreateEmailVerificationParams{ID: uuid.New(), UserID: user.ID, Token: token, Email: user.Email, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Duration(30 * time.Minute)), Valid: true}})
				return insert_err
			})

			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			helpers.ResendEmailVerificationTemplate(user.Email, ev.Token)

			csrf := c.Get("csrf").(string)

			html := helpers.MustRenderHTML(components.EmailVerification(user.Email, csrf, "/verification/manual"))

			return c.Blob(http.StatusCreated, "text/html", html)
		}

		if !helpers.CheckPasswordHash(payload.Password, user.PasswordHash) {
			return herr.Why("invalid password").Handle(c.Response(), http.StatusUnauthorized, errors.New("invalid password"))
		}

		if user.Role == string(enums.Roles.DEVELOPER) && !user.IsEmailVerified {
			csrf := c.Get("csrf").(string)

			html := helpers.MustRenderHTML(components.DevResetCard(csrf, user.ID.String()))

			return c.Blob(http.StatusOK, "text/html", html)
		}

		if user.TwofaEnabled {
			csrf := c.Get("csrf").(string)

			challengeID, err := helpers.GenerateProofUUIDV4(database.IsPKCollision("pending_auth_challenges_pkey"), func(u uuid.UUID) error {
				_, err = repo.CreatePendingAuthChallenge(ctx, repository.CreatePendingAuthChallengeParams{
					ID:         u,
					UserID:     user.ID,
					Mode:       enums.TwofaModes.CHALLENGE.String(),
					Secret:     nil,
					RememberMe: payload.Remeber == "on",
					ExpiresAt: pgtype.Timestamptz{
						Time:  time.Now().Add(time.Duration(time.Minute * 10)),
						Valid: true,
					},
				})

				return err
			})

			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			html := helpers.MustRenderHTML(components.TwoFACheck(challengeID.String(), csrf))

			return c.Blob(http.StatusOK, "text/html", html)
		}

		if _, err = helpers.GenerateProofUUIDV7(database.IsPKCollision("sessions_pkey"), func(id uuid.UUID) error {
			err = auth.CreateSession(c.Response(), c.Request(), repo, id, user.ID, payload.Remeber == "on")
			return err
		}); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		// if err := auth.SetSessionUser(c.Response(), c.Request(), auser, payload.Remeber == "on"); err != nil {
		// 	return helpers.SendReturnedHTMLErrorMessage(c, helpers.ErrorMessage{Error: helpers.GenericError{Code: http.StatusInternalServerError, UserMessage: "failed to set session", Message: fmt.Errorf("failed to set session: %v", err).Error()}, Box: enums.Boxes.TOAST_TR, Persistance: "5000"}, nil)
		// }

		_ = repo.UpdateUserLastLogin(ctx, user.ID)

		c.Response().Header().Set("HX-Redirect", "/dashboard")
		return c.NoContent(http.StatusOK)
	}
}

func SessionLoginTwoFACheck() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("two factor authentication", "SessionLoginTwoFACheck", c.Request().Header.Get("X-Request-ID"))
		challengeID := c.FormValue("token")
		otp := c.FormValue("otp")

		if otp == "" || challengeID == "" {
			return herr.Handle(c.Response(), http.StatusBadRequest, errors.New("invalid form data"))
		}

		challengeUUID, err := uuid.Parse(challengeID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("could not parse ID: %v", err))
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		challenge, err := repo.GetPendingAuthChallenge(ctx, challengeUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("challenge not found: %v", err))
		}

		secrets, err := repo.GetUser2FASecret(ctx, challenge.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("user not found: %v", err))
		}

		key, err := helpers.ParseBase64Key(boot.Environment.TwofaKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("invalid key: %v", err))
		}

		decriptedTwofaSecret, err := helpers.Decrypt(*secrets.TwofaSecret, key)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("invalid key: %v", err))
		}

		if !totp.Validate(otp, string(decriptedTwofaSecret)) {
			return herr.Handle(c.Response(), http.StatusUnauthorized, errors.New("totp validation failed"))
		}

		if _, err = helpers.GenerateProofUUIDV7(database.IsPKCollision("sessions_pkey"), func(id uuid.UUID) error {
			err = auth.CreateSession(c.Response(), c.Request(), repo, id, challenge.UserID, challenge.RememberMe)
			return err
		}); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to set session: %v", err))
		}

		// if err := auth.SetSessionUser(c.Response(), c.Request(), auth.AuthenticatedSessionUser{
		// 	ID:       challenge.UserID.String(),
		// 	Username: secrets.Username,
		// 	Email:    secrets.Email,
		// 	Role:     secrets.Role,
		// }, challenge.RememberMe); err != nil {
		// 	return helpers.SendReturnedHTMLErrorMessage(c, helpers.ErrorMessage{Error: helpers.GenericError{Code: http.StatusInternalServerError, UserMessage: "failed to set session", Message: fmt.Errorf("failed to set session: %v", err).Error()}, Box: enums.Boxes.TOAST_TR, Persistance: "5000"}, nil)
		// }

		err = repo.UpdateUserLastLogin(ctx, challenge.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not update user last login: %v", err))
		}

		err = repo.DeletePendingAuthChallenge(ctx, challengeUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not delete pending auth challenge: %v", err))
		}

		c.Response().Header().Set("HX-Redirect", "/dashboard")
		return c.NoContent(http.StatusOK)
	}
}

func TwoFAResetForm() echo.HandlerFunc {
	return func(c echo.Context) error {
		data := config.GetDefaultSite(c.Request())

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		challengeID := c.QueryParam("token")

		html := helpers.MustRenderHTML(views.TwoFAReset(data, challengeID))
		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func TwoFAReset() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("two factor authentication reset", "TwoFAReset", c.Request().Header.Get("X-Request-ID"))
		challengeID := c.FormValue("token")
		code := c.FormValue("reset_code")

		if code == "" || challengeID == "" {
			return herr.Handle(c.Response(), http.StatusBadRequest, errors.New("invalid form data"))
		}

		challengeUUID, err := uuid.Parse(challengeID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		challenge, err := repo.GetPendingAuthChallenge(ctx, challengeUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("challenge not found: %v", err))
		}

		count, err := repo.CountUnusedBackupCodesForUser(ctx, challenge.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		if count <= 0 {
			return herr.Handle(c.Response(), http.StatusConflict, fmt.Errorf("no more backup codes available: %v", err))
		}

		h, err := blake2b.New512(nil)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		h.Write([]byte(code))
		codeHash := hex.EncodeToString(h.Sum(nil))

		backupCode, err := repo.GetBackupCodeByHash(ctx, string(codeHash))
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		if backupCode.Used {
			return herr.Handle(c.Response(), http.StatusConflict, errors.New("code has already been used"))
		}

		user, err := repo.GetUserByID(ctx, challenge.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		key, err := totp.Generate(totp.GenerateOpts{
			Issuer:      config.GetDefaultSite(c.Request()).AppName,
			AccountName: user.Email,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		encryptionKey, err := helpers.ParseBase64Key(boot.Environment.TwofaKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		encryptedTwofaSecret, err := helpers.Encrypt([]byte(key.Secret()), encryptionKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		_, err = repo.PromoteChallengeToRegistration(ctx, repository.PromoteChallengeToRegistrationParams{
			ID:     challengeUUID,
			Secret: &encryptedTwofaSecret,
		})

		image, err := key.Image(200, 200)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		var buf bytes.Buffer
		if err := png.Encode(&buf, image); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		pngBytes := buf.Bytes()
		base64Str := base64.StdEncoding.EncodeToString(pngBytes)
		qr_code := fmt.Sprintf("data:image/png;base64,%s", base64Str)

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.TwoFAQRCodeResetCard(components.TwoFAResetProps{
			QRCodeDataURL: qr_code,
			Secret:        key.Secret(),
			CSRF:          csrf,
			Token:         challengeID,
			BackupCodeID:  backupCode.ID.String(),
		}))

		return c.Blob(http.StatusAccepted, "text/html", html)
	}
}

func TwoFACancelReset() echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("HX-Redirect", "/auth")
		return c.NoContent(http.StatusOK)
	}
}

func TwoFAVerifyReset() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("two factor verify reset", "TwoFAVerifyReset", c.Request().Header.Get("X-Request-ID"))
		challengeID := c.FormValue("token")
		otp := c.FormValue("otp")
		code := c.FormValue("code")

		if challengeID == "" || otp == "" || code == "" {
			return herr.Handle(c.Response(), http.StatusBadRequest, errors.New("invalid form data"))
		}

		challengeUUID, err := uuid.Parse(challengeID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		codeUUID, err := uuid.Parse(code)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse code UUID: %v", err))
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		challenge, err := repo.GetPendingAuthChallenge(ctx, challengeUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("challenge not found: %v", err))
		}

		key, err := helpers.ParseBase64Key(boot.Environment.TwofaKey)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not parse key: %v", err))
		}

		encrypted_totp_secret := challenge.Secret

		totp_secret, err := helpers.Decrypt(*encrypted_totp_secret, key)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not decrypt totp secret: %v", err))
		}

		if !totp.Validate(otp, string(totp_secret)) {
			return herr.Handle(c.Response(), http.StatusUnauthorized, errors.New("totp validation failed"))
		}

		err = repo.EnableUser2FA(ctx, repository.EnableUser2FAParams{
			TwofaSecret: encrypted_totp_secret,
			ID:          challenge.UserID,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not enable 2fa: %v", err))
		}

		err = repo.MarkBackupCodeUsed(ctx, codeUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not mark backup code used: %v", err))
		}

		err = repo.DeletePendingAuthChallenge(ctx, challengeUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not delete pending auth challenge: %v", err))
		}
		c.Response().Header().Set("HX-Redirect", "/auth")
		return c.NoContent(http.StatusOK)
	}
}

func TwoFARestoreForm() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("two factor restore form", "TwoFARestoreForm", c.Request().Header.Get("X-Request-ID"))
		data := config.GetDefaultSite(c.Request())

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		challengeID := c.QueryParam("token")

		challengeUUID, err := uuid.Parse(challengeID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		code, err := helpers.GenerateBase62Token(8)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to generate token: %v", err))
		}
		slog.Debug("Generated code", slog.String("code", code))

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		challenge, err := repo.GetPendingAuthChallenge(ctx, challengeUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("challenge not found: %v", err))
		}

		user, err := repo.GetUserByID(ctx, challenge.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("user not found: %v", err))
		}

		var ev *repository.CreateEmailVerificationRow

		_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("email_verifications_pkey"), func(id uuid.UUID) error {
			var insert_err error
			ev, insert_err = repo.CreateEmailVerification(ctx, repository.CreateEmailVerificationParams{ID: uuid.New(), UserID: user.ID, Token: code, Email: user.Email, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Duration(30 * time.Minute)), Valid: true}})
			return insert_err
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to Create email verification: %v", err))
		}

		helpers.ResendEmailVerificationTemplate(user.Email, ev.Token)
		html := helpers.MustRenderHTML(views.TwoFARestore(data))
		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func TwoFARestore() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("two factor restore", "TwoFARestore", c.Request().Header.Get("X-Request-ID"))
		resetCode := c.FormValue("reset_code")
		if resetCode == "" {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("reset code is required"))
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		verification, err := repo.GetEmailVerificationByToken(ctx, resetCode)

		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to get email verification: %v", err))
		}

		if verification.ExpiresAt.Time.Before(time.Now()) {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("token expired: %v", err))
		}

		err = repo.VerifyUserEmail(ctx, verification.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to verify email: %v", err))
		}

		err = repo.MarkEmailVerificationUsed(ctx, verification.Token)

		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark email verification used: %v", err))
		}

		err = repo.DeleteUserBackupCodes(ctx, verification.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to delete user backup codes: %v", err))
		}

		err = repo.DisableUser2FA(ctx, verification.UserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to disable user 2FA: %v", err))
		}

		c.Response().Header().Set("HX-Redirect", "/auth")
		return c.NoContent(http.StatusOK)

	}
}

func SessionLogout() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("logout", "SessionLogout", c.Request().Header.Get("X-Request-ID"))
		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)
		if err := auth.RevokeCurrentSession(c.Response(), c.Request(), repo); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to logout: %v", err))
		}

		// if err := auth.ClearSession(c.Response(), c.Request()); err != nil {
		// 	return helpers.SendReturnedHTMLErrorMessage(c, helpers.ErrorMessage{Error: helpers.GenericError{Code: http.StatusInternalServerError, UserMessage: "failed to logout", Message: fmt.Errorf("failed to logout: %v", err).Error()}, Box: enums.Boxes.TOAST_TR, Persistance: "5000"}, nil)
		// }
		c.Response().Header().Set("HX-Redirect", "/")
		return c.NoContent(http.StatusOK)
	}
}
