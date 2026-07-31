package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/blake2b"
)

type AuthenticatedSessionUser struct {
	ID             string
	Username       string
	Email          string
	Role           string
	FullName       string
	Title          string
	IsActive       bool
	TwoFAEnabled   bool
	Remember       bool
	SessionID      uuid.UUID
	LastActivityAt time.Time
}

// Server Side DB Stored SESSIONS

const sessionCookieName = "sid"

func generateSessionToken() (token string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	token = base64.URLEncoding.EncodeToString(b)

	h, err := blake2b.New512(nil)
	if err != nil {
		return
	}
	h.Write([]byte(token))
	hash = hex.EncodeToString(h.Sum(nil))
	return
}

func sessionMaxAge(remember bool) int {
	if boot.Environment.GoEnv == enums.Environments.DEVELOPMENT {
		if remember {
			return 0 // session-only in dev
		}
		return 5 * 60 // 5 min
	}
	if remember {
		return 86400 * 7 * 52 // 1 year
	}
	return 86400 * 7 // 1 week
}

func CreateSession(w http.ResponseWriter, r *http.Request, repo *repository.Queries, id uuid.UUID, userID uuid.UUID, remember bool) error {
	token, hash, err := generateSessionToken()
	if err != nil {
		return err
	}

	maxAge := sessionMaxAge(remember)

	var expiresAt time.Time
	if maxAge == 0 {
		expiresAt = time.Now().Add(24 * time.Hour)
	} else {
		expiresAt = time.Now().Add(time.Duration(maxAge) * time.Second)
	}

	userAgent := r.UserAgent()

	_, err = repo.CreateUserSession(r.Context(), repository.CreateUserSessionParams{
		ID:               id,
		UserID:           userID,
		SessionTokenHash: hash,
		IpAddress:        &r.RemoteAddr,
		UserAgent:        &userAgent,
		RememberMe:       remember,
		ExpiresAt: pgtype.Timestamptz{
			Time:  expiresAt,
			Valid: true,
		},
	})
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token, // raw token, never the hash
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   boot.Environment.GoEnv != enums.Environments.DEVELOPMENT,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func GetActiveSession(r *http.Request, repo *repository.Queries) (*AuthenticatedSessionUser, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, nil
	}

	sum, err := blake2b.New512(nil)
	if err != nil {
		return nil, err
	}
	sum.Write([]byte(cookie.Value))
	hash := hex.EncodeToString(sum.Sum(nil))

	row, err := repo.GetActiveSession(r.Context(), hash)
	if err != nil {
		return nil, err
	}

	return &AuthenticatedSessionUser{
		ID:             row.UserID.String(),
		Username:       row.Username,
		Email:          row.Email,
		Role:           row.Role,
		FullName:       row.FullName,
		Title:          row.Title,
		IsActive:       row.IsActive,
		TwoFAEnabled:   row.TwofaEnabled,
		Remember:       row.RememberMe,
		SessionID:      row.ID,
		LastActivityAt: row.LastActivityAt.Time,
	}, nil
}

func TouchSession(r *http.Request, repo *repository.Queries, sessionID uuid.UUID, lastActivity time.Time) {
	go func() {
		if time.Since(lastActivity) > 5*time.Minute {
			_ = repo.TouchSession(r.Context(), sessionID)
		}
	}()
}

func RevokeSessionByID(r *http.Request, repo *repository.Queries, sessionID uuid.UUID) error {
	return repo.RevokeSession(r.Context(), sessionID)
}

func RevokeCurrentSession(w http.ResponseWriter, r *http.Request, repo *repository.Queries) error {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil // already gone
	}

	h, err := blake2b.New512(nil)
	if err != nil {
		return err
	}
	h.Write([]byte(cookie.Value))
	hash := hex.EncodeToString(h.Sum(nil))

	if err := repo.RevokeSessionByHash(r.Context(), hash); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   boot.Environment.GoEnv != enums.Environments.DEVELOPMENT,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func RevokeAllSessions(w http.ResponseWriter, r *http.Request, repo *repository.Queries, userID uuid.UUID) error {
	if err := repo.RevokeAllUserSessions(r.Context(), userID); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   boot.Environment.GoEnv != enums.Environments.DEVELOPMENT,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func RevokeOtherSessions(r *http.Request, repo *repository.Queries, userID uuid.UUID, currentSessionID uuid.UUID) error {
	return repo.RevokeAllUserSessionsExcept(r.Context(), repository.RevokeAllUserSessionsExceptParams{
		UserID: userID,
		ID:     currentSessionID,
	})
}
