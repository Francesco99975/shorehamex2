package controllers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
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
	"github.com/Francesco99975/shorehamex2/internal/tools"
	"github.com/Francesco99975/shorehamex2/views"
	"github.com/Francesco99975/shorehamex2/views/components"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
)

func Settings(tab string) echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("settings", "Settings", c.Request().Header.Get("X-Request-ID"))
		if tab == "" {
			tab = "profile"
		}

		data := config.GetDefaultSite(c.Request())

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		auser, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		if auser == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		data.CurrentUser = auser

		userUUID, err := uuid.Parse(auser.ID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("failed to parse user ID: %v", err))
		}

		user, err := repo.GetUserByID(ctx, userUUID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("failed to get user by ID: %v", err))
		}

		switch tab {
		case "profile":
			profileProps := components.ProfileProps{
				Username:      user.Username,
				Email:         user.Email,
				EmailVerified: user.IsEmailVerified,
				Initials:      strings.Split(user.Username, "")[0],
				UserID:        user.ID.String(),
				Role:          user.Role,
				Created:       user.CreatedAt.Time.Format("January 2, 2006"),
				LastLogin:     user.LastLogin.Time.Format("January 2, 2006"),
				CSRF:          c.Get("csrf").(string),
			}

			html := helpers.MustRenderHTML(views.SettingsProfile(data, profileProps))

			return c.Blob(http.StatusOK, "text/html", html)
		case "security":

			sessions, err := repo.GetActiveSessionsByUser(ctx, userUUID)
			if err != nil {
				return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("failed to get active sessions: %v", err))
			}

			slog.Debug("sessions", slog.Any("sessions", sessions))

			sessionsInfo := helpers.MapSlice(sessions, func(session *repository.Session) components.SessionInfo {
				return components.SessionInfo{
					Device:    helpers.GetUaDeviceType(*session.UserAgent),
					Browser:   helpers.GetUaBrowser(*session.UserAgent),
					OS:        helpers.GetUaOS(*session.UserAgent),
					LastUsed:  session.LastActivityAt.Time.Format(time.RFC850),
					Expires:   session.ExpiresAt.Time.Format(time.RFC850),
					IsCurrent: session.ID == auser.SessionID,
					SessionID: session.ID.String(),
				}
			})

			slog.Debug("sessionsInfo", slog.Any("sessionsInfo", sessionsInfo))

			securityProps := components.SecurityProps{
				TwoFAEnabled: user.TwofaEnabled,
				Sessions:     sessionsInfo,
				CSRF:         c.Get("csrf").(string),
			}

			html := helpers.MustRenderHTML(views.SettingsSecurity(data, securityProps))

			return c.Blob(http.StatusOK, "text/html", html)
		case "account":
			accountProps := components.AccountProps{
				IsActive:     user.IsActive,
				TwoFAEnabled: user.TwofaEnabled,
				UserEmail:    user.Email,
				CSRF:         c.Get("csrf").(string),
			}

			html := helpers.MustRenderHTML(views.SettingsAccount(data, accountProps))

			return c.Blob(http.StatusOK, "text/html", html)
		case "users":
			search := c.QueryParam("search")
			role := c.QueryParam("role_filter")
			pageStr := c.QueryParam("page")
			var page int

			page, err := strconv.Atoi(pageStr)
			if err != nil {
				page = 1
			}

			totalUsers, err := repo.GetUsersCount(ctx)
			if err != nil {
				return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("failed to get users count: %v", err))
			}

			rawUsers, err := repo.SearchUsers(ctx, repository.SearchUsersParams{
				Column1: search,
				Column2: role,
				Limit:   int32(boot.Environment.PaginationWindow),
				Column4: page,
			})
			if err != nil {
				return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("failed to search users: %v", err))
			}

			filteredUsers := helpers.FilteredSlice(rawUsers, func(user *repository.SearchUsersRow) bool {
				return auser.ID != user.ID.String() &&
					(auser.Role == enums.Roles.DEVELOPER.String() ||
						user.Role != enums.Roles.DEVELOPER.String())
			})

			totalUsers = totalUsers - int64(len(rawUsers)-len(filteredUsers))

			users := helpers.MapSlice(filteredUsers, func(user *repository.SearchUsersRow) components.UserInfo {

				status := "Active"
				if !user.IsActive {
					status = "Inactive"
				}
				return components.UserInfo{
					ID:        user.ID.String(),
					Username:  user.Username,
					Email:     user.Email,
					Verified:  user.IsEmailVerified,
					Initials:  strings.Split(user.Username, "")[0],
					Role:      user.Role,
					Status:    status,
					TwoFA:     user.TwofaEnabled,
					LastLogin: user.LastLogin.Time.Format(time.RFC3339),
					Gradient:  "primary",
					CanEdit:   auth.CanManageUser(enums.Role(auser.Role), enums.ActEdit, enums.Role(user.Role)),
					CanDelete: auth.CanManageUser(enums.Role(auser.Role), enums.ActDelete, enums.Role(user.Role)),
				}
			})

			usersProps := components.UsersProps{
				Users:      users,
				TotalUsers: int(totalUsers),
				Viewer:     enums.Role(auser.Role),
				Page:       1,
				PerPage:    boot.Environment.PaginationWindow,
				CSRF:       c.Get("csrf").(string),
			}

			html := helpers.MustRenderHTML(views.SettingsUsers(data, usersProps))

			return c.Blob(http.StatusOK, "text/html", html)

		default:
			return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("invalid tab: %s", tab))
		}

	}
}

func UpdateUsername() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("updating username", "UpdateUsername", c.Request().Header.Get("X-Request-ID"))
		username := c.FormValue("username")

		if username == "" {
			return herr.Handle(c.Response(), http.StatusNotFound, errors.New("invalid form data"))
		}

		username = strings.ToLower(username)

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("unable to get transaction: %v", err))
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

		_, err = repo.UpdateUserUsername(ctx, repository.UpdateUserUsernameParams{Username: username, ID: userUUID})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not update username or email: %v", err))
		}

		tools.SetToastTrigger(c.Response(), enums.SuccessToast, "Successfully updated username")
		return c.NoContent(http.StatusAccepted)
	}
}

func UpdateEmail() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("updating email", "UpdateEmail", c.Request().Header.Get("X-Request-ID"))
		var payload models.ChangeEmail

		if err := c.Bind(&payload); err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("invalid form data: %v", err))
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		exists, err := repo.ExistsUserWithEmail(ctx, payload.Email)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to check if email exists: %v", err))
		}

		if exists {
			return herr.Handle(c.Response(), http.StatusConflict, errors.New("email already exists"))
		}

		user, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		if user == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		userUUID, err := uuid.Parse(user.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		if payload.Email == user.Email {
			return herr.Handle(c.Response(), http.StatusBadRequest, errors.New("email did not change"))
		}

		token, err := helpers.GenerateBase62Token(8)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to generate token: %v", err))
		}

		_, err = repo.CreateEmailVerification(ctx, repository.CreateEmailVerificationParams{ID: uuid.New(), UserID: userUUID, Token: token, Email: payload.Email, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Duration(30 * time.Minute)), Valid: true}})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to generate token: %v", err))
		}

		helpers.ResendEmailVerificationTemplate(payload.Email, token)

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.EmailVerification(payload.Email, csrf, "/verification/update"))

		tools.SetToastTrigger(c.Response(), enums.WarningToast, "Email needs to be verified")
		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func UpdateManualEmailVerification() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("updating email", "UpdateManualEmailVerification", c.Request().Header.Get("X-Request-ID"))
		var payload models.VerifyEmailRequest
		err := c.Bind(&payload)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		err = payload.Validate()
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		verification, err := repo.GetEmailVerificationByToken(ctx, payload.Token)

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

		user, err := repo.UpdateUserEmail(ctx, repository.UpdateUserEmailParams{
			Email: verification.Email,
			ID:    verification.UserID,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark get user: %v", err))
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.ChangeUserEmailForm(user.Email, user.IsEmailVerified, csrf))

		tools.SetToastTrigger(c.Response(), enums.SuccessToast, "Email updated successfully")
		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func UpdateUserPassword() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("update user password", "UpdateUserPassword", c.Request().Header.Get("X-Request-ID"))
		var payload models.ChangePasswordRequest

		if err := c.Bind(&payload); err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("invalid form data: %v", err))
		}

		slog.Debug("Change password payload", slog.Any("payload", payload))

		err := payload.Validate(1)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data on validate: %v", err))
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
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		hash, err := repo.GetPasswordHash(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("unable to get password hash: %v", err))
		}

		if !helpers.CheckPasswordHash(payload.CurrentPassword, hash) {
			return herr.Handle(c.Response(), http.StatusUnauthorized, fmt.Errorf("invalid password: %v", err))
		}

		hashedPassword, err := helpers.HashPassword(payload.NewPassword)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to hash password: %v", err))
		}

		err = repo.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
			ID:           userUUID,
			PasswordHash: hashedPassword,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to update user password: %v", err))
		}

		tools.SetToastTrigger(c.Response(), enums.SuccessToast, "Successfully updated user password")
		return c.NoContent(http.StatusAccepted)

	}
}

func RevokeSession() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("revokeing session", "RevokeSession", c.Request().Header.Get("X-Request-ID"))
		sessionID := c.Param("id")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get transaction: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		sessionUUID, err := uuid.Parse(sessionID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to parse session ID: %v", err))
		}

		err = repo.RevokeSession(ctx, sessionUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to revoke session: %v", err))
		}

		return c.NoContent(http.StatusOK)
	}
}

func DeactivateUser() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("deactivating user", "DeactivateUser", c.Request().Header.Get("X-Request-ID"))
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

		err = repo.DeactivateUser(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not deactivate user: %v", err))
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.AccountStatusCard(false, true))
		html = append(html, helpers.MustRenderHTML(components.DeactivateSection(false, csrf, true))...)

		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func ActivateUser() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("activating user", "ActivateUser", c.Request().Header.Get("X-Request-ID"))
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

		err = repo.ReactivateUser(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not reactivate user: %v", err))
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.AccountStatusCard(true, true))
		html = append(html, helpers.MustRenderHTML(components.DeactivateSection(true, csrf, true))...)

		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func PermanentlyDeleteUser() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("permanently deleting user", "PermanentlyDeleteUser", c.Request().Header.Get("X-Request-ID"))

		password := c.FormValue("password")
		otp := c.FormValue("otp")

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

		if auser.TwoFAEnabled {
			secrets, err := repo.GetUser2FASecret(ctx, userUUID)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("user not found: %v", err))
			}

			if !helpers.CheckPasswordHash(password, secrets.PasswordHash) {
				return herr.Handle(c.Response(), http.StatusUnauthorized, fmt.Errorf("invalid credentials: %v", err))
			}

			encryptionKey, err := helpers.ParseBase64Key(boot.Environment.TwofaKey)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not parse twofa key: %v", err))
			}

			totp_secret, err := helpers.Decrypt(*secrets.TwofaSecret, encryptionKey)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not decrypt twofa secret: %v", err))
			}

			if !totp.Validate(otp, string(totp_secret)) {
				return herr.Handle(c.Response(), http.StatusUnauthorized, fmt.Errorf("unauthorized: invalid code"))
			}
		} else {
			hash, err := repo.GetPasswordHash(ctx, userUUID)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("user not found: %v", err))
			}

			if !helpers.CheckPasswordHash(password, hash) {
				return herr.Handle(c.Response(), http.StatusUnauthorized, errors.New("invalid credentials"))
			}
		}

		if err := auth.RevokeCurrentSession(c.Response(), c.Request(), repo); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to clear session: %v", err))
		}

		err = repo.DeleteUser(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not delete user: %v", err))
		}

		c.Response().Header().Set("HX-Redirect", "/")
		return c.NoContent(http.StatusOK)
	}
}
func GetUser() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("fetching user", "GetUser", c.Request().Header.Get("X-Request-ID"))
		id := c.Param("id")
		userID, err := uuid.Parse(id)
		if err != nil {
			return herr.HandleEchoPage(http.StatusBadRequest, fmt.Errorf("invalid ID: %v", err))
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		auser, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
		}
		if auser == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		user, err := repo.GetUserByID(ctx, userID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusNotFound, fmt.Errorf("unable to get user: %v", err))
		}

		userActiveSessions, err := repo.GetActiveSessionsByUser(ctx, userID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("failed to get user's active sessions: %v", err))
		}

		sessionsInfo := helpers.MapSlice(userActiveSessions, func(session *repository.Session) components.SessionInfo {
			return components.SessionInfo{
				Device:    helpers.GetUaDeviceType(*session.UserAgent),
				Browser:   helpers.GetUaBrowser(*session.UserAgent),
				OS:        helpers.GetUaOS(*session.UserAgent),
				LastUsed:  session.LastActivityAt.Time.Format(time.RFC850),
				Expires:   session.ExpiresAt.Time.Format(time.RFC850),
				IsCurrent: session.ID == auser.SessionID,
				SessionID: session.ID.String(),
			}
		})

		status := "Active"
		if !user.IsActive {
			status = "Inactive"
		}
		userInfo := components.UserInfo{
			ID:        user.ID.String(),
			Username:  user.Username,
			Email:     user.Email,
			Sessions:  sessionsInfo,
			Verified:  user.IsEmailVerified,
			Initials:  strings.Split(user.Username, "")[0],
			Role:      user.Role,
			Status:    status,
			TwoFA:     user.TwofaEnabled,
			LastLogin: user.LastLogin.Time.Format(time.RFC3339),
			Gradient:  "primary",
			CanEdit:   auth.CanManageUser(enums.Role(auser.Role), enums.ActEdit, enums.Role(user.Role)),
			CanDelete: auth.CanManageUser(enums.Role(auser.Role), enums.ActDelete, enums.Role(user.Role)),
		}

		data := config.GetDefaultSite(c.Request())

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		html := helpers.MustRenderHTML(views.UserDetails(data, userInfo))

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func CreateUser() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("creating user", "CreateUser", c.Request().Header.Get("X-Request-ID"))
		var payload models.CreateNewUser

		if err := c.Bind(&payload); err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		err := payload.Validate(0)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to open database on signup: %v", err))
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

		if auser.Role == enums.Roles.USER.String() {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		if auser.Role == enums.Roles.ADMIN.String() && payload.Role == enums.Roles.DEVELOPER.String() {
			tools.SetToastTrigger(c.Response(), enums.WarningToast, "You are not allowed to create a DEVELOPER user")
			return c.NoContent(http.StatusBadRequest)
		}

		hashedPassword, err := helpers.HashPassword(payload.Password)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to hash password: %v", err))
		}

		var createdUser *repository.CreateUserRow

		_, err = helpers.GenerateProofUUIDV7(database.IsPKCollision("users_pkey"), func(id uuid.UUID) error {
			var insert_err error
			createdUser, insert_err = repo.CreateUser(ctx, repository.CreateUserParams{
				ID:           id,
				Username:     payload.Username,
				Email:        payload.Email,
				Role:         payload.Role,
				PasswordHash: hashedPassword,
			})
			return insert_err
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to generate UUID: %v", err))
		}

		var userStatus string = "Inactive"
		if createdUser.IsActive {
			userStatus = "Active"
		}

		userInfo := components.UserInfo{
			ID:        createdUser.ID.String(),
			Username:  createdUser.Username,
			Email:     createdUser.Email,
			Verified:  createdUser.IsEmailVerified,
			Initials:  strings.Split(createdUser.Username, "")[0],
			Role:      createdUser.Role,
			Status:    userStatus,
			TwoFA:     createdUser.TwofaEnabled,
			Gradient:  "primary",
			CanEdit:   auth.CanManageUser(enums.Role(auser.Role), enums.ActEdit, enums.Role(payload.Role)),
			CanDelete: auth.CanManageUser(enums.Role(auser.Role), enums.ActDelete, enums.Role(payload.Role)),
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.SettingsUserItem(userInfo, csrf))

		tools.SetToastTrigger(c.Response(), enums.SuccessToast, "User created successfully")

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func UpdateUser() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("updating user", "UpdateUser", c.Request().Header.Get("X-Request-ID"))
		id := c.Param("id")

		userUUID, err := uuid.Parse(id)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		var payload models.UpdateUserRequest

		if err := c.Bind(&payload); err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		err = payload.Validate(0)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid form data: %v", err))
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to begin transaction: %v", err))
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

		if auser.Role == enums.Roles.USER.String() {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		if auser.Role == enums.Roles.ADMIN.String() && payload.Role == enums.Roles.DEVELOPER.String() {
			tools.SetToastTrigger(c.Response(), enums.WarningToast, "You are not allowed to update a DEVELOPER user")
			return c.NoContent(http.StatusBadRequest)
		}

		isActive := payload.Active == "on"

		var updatedUser *repository.UpdateUserRow
		if payload.Password != "" {
			hashedPassword, err := helpers.HashPassword(payload.Password)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to hash password: %v", err))
			}

			updatedUser, err = repo.UpdateUser(ctx, repository.UpdateUserParams{
				ID:       userUUID,
				Username: payload.Username,
				Role:     payload.Role,
				Email:    payload.Email,
				FullName: payload.FullName,
				Title:    payload.Title,
				IsActive: isActive,
				Column8:  hashedPassword,
			})
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to update user: %v", err))
			}

		} else {
			updatedUser, err = repo.UpdateUser(ctx, repository.UpdateUserParams{
				ID:       userUUID,
				Username: payload.Username,
				Role:     payload.Role,
				Email:    payload.Email,
				IsActive: isActive,
			})
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to update user: %v", err))
			}
		}

		var userStatus string = "Inactive"
		if updatedUser.IsActive {
			userStatus = "Active"
		}

		userInfo := components.UserInfo{
			ID:        updatedUser.ID.String(),
			Username:  updatedUser.Username,
			Email:     updatedUser.Email,
			Verified:  updatedUser.IsEmailVerified,
			Initials:  strings.Split(updatedUser.Username, "")[0],
			Role:      updatedUser.Role,
			Status:    userStatus,
			TwoFA:     updatedUser.TwofaEnabled,
			Gradient:  "primary",
			CanEdit:   auth.CanManageUser(enums.Role(auser.Role), enums.ActEdit, enums.Role(payload.Role)),
			CanDelete: auth.CanManageUser(enums.Role(auser.Role), enums.ActDelete, enums.Role(payload.Role)),
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.SettingsUserItem(userInfo, csrf))

		tools.SetToastTrigger(c.Response(), enums.SuccessToast, "User updated successfully")

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func ReactivateUserAsAdmin() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("reactivating user as admin", "ReactivateUserAsAdmin", c.Request().Header.Get("X-Request-ID"))

		userID := c.Param("id")

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to begin transaction: %v", err))
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

		userUUID, err := uuid.Parse(userID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		err = repo.ReactivateUser(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not reactivate user: %v", err))
		}

		user, err := repo.GetUserByID(ctx, userUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not get user by ID: %v", err))
		}

		tools.SetToastTrigger(c.Response(), enums.SuccessToast, "User reactivated successfully")

		userInfo := components.UserInfo{
			ID:        user.ID.String(),
			Username:  user.Username,
			Email:     user.Email,
			Verified:  user.IsEmailVerified,
			Initials:  strings.Split(user.Username, "")[0],
			Role:      user.Role,
			Status:    "Active",
			TwoFA:     user.TwofaEnabled,
			Gradient:  "primary",
			CanEdit:   auth.CanManageUser(enums.Role(auser.Role), enums.ActEdit, enums.Role(user.Role)),
			CanDelete: auth.CanManageUser(enums.Role(auser.Role), enums.ActDelete, enums.Role(user.Role)),
			LastLogin: user.LastLogin.Time.Format(time.RFC822Z),
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.SettingsUserItem(userInfo, csrf))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func DeleteUser() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("deleting user", "DeleteUser", c.Request().Header.Get("X-Request-ID"))

		password := c.FormValue("password")

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to begin transaction: %v", err))
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

		auserUUID, err := uuid.Parse(auser.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("could not parse ID: %v", err))
		}

		hash, err := repo.GetPasswordHash(ctx, auserUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusNotFound, fmt.Errorf("unable to find auser password: %v", err))
		}

		if !helpers.CheckPasswordHash(password, hash) {
			return herr.Handle(c.Response(), http.StatusUnauthorized, fmt.Errorf("invalid auser password: %v", err))
		}

		id := c.Param("id")
		userID, err := uuid.Parse(id)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not parse ID to delete user: %v", err))
		}

		err = repo.DeleteUser(ctx, userID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("could not delete user: %v", err))
		}

		tools.SetToastTrigger(c.Response(), enums.SuccessToast, "Successfully deleted user")
		return c.NoContent(http.StatusCreated)
	}
}

func SearchUsers() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("searching users", "SearchUsers", c.Request().Header.Get("X-Request-ID"))
		search := c.QueryParam("search")
		role := c.QueryParam("role_filter")
		pageStr := c.QueryParam("page")
		var page int

		page, err := strconv.Atoi(pageStr)
		if err != nil {
			page = 1
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to get transaction: %v", err))
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

		rawUsers, err := repo.SearchUsers(ctx, repository.SearchUsersParams{
			Column1: search,
			Column2: role,
			Limit:   int32(boot.Environment.PaginationWindow),
			Column4: page,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get users: %v", err))
		}

		filteredUsers := helpers.FilteredSlice(rawUsers, func(user *repository.SearchUsersRow) bool {
			return auser.ID != user.ID.String() &&
				(auser.Role == enums.Roles.DEVELOPER.String() ||
					user.Role != enums.Roles.DEVELOPER.String())
		})

		totalUsers, err := repo.GetUserCountBySearch(ctx, repository.GetUserCountBySearchParams{
			Column1: search,
			Column2: role,
		})

		totalUsers = totalUsers - int64(len(rawUsers)-len(filteredUsers))

		users := helpers.MapSlice(filteredUsers, func(user *repository.SearchUsersRow) components.UserInfo {

			status := "Active"
			if !user.IsActive {
				status = "Inactive"
			}
			return components.UserInfo{
				ID:        user.ID.String(),
				Username:  user.Username,
				Email:     user.Email,
				Verified:  user.IsEmailVerified,
				Initials:  strings.Split(user.Username, "")[0],
				Role:      user.Role,
				Status:    status,
				TwoFA:     user.TwofaEnabled,
				LastLogin: user.LastLogin.Time.Format(time.RFC3339),
				Gradient:  "primary",
				CanEdit:   auth.CanManageUser(enums.Role(auser.Role), enums.ActEdit, enums.Role(user.Role)),
				CanDelete: auth.CanManageUser(enums.Role(auser.Role), enums.ActDelete, enums.Role(user.Role)),
			}
		})

		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get users: %v", err))
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.UserList(users, csrf))
		html = append(html, helpers.MustRenderHTML(components.UsersPagination(int(totalUsers), page, boot.Environment.PaginationWindow, true))...)

		return c.Blob(http.StatusOK, "text/html", html)
	}
}
