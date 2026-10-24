package controllers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Francesco99975/shorehamex2/internal/auth"
	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/Francesco99975/shorehamex2/internal/database"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/internal/httperr"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/Francesco99975/shorehamex2/views"
	"github.com/Francesco99975/shorehamex2/views/dashboard"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

		data := config.GetDefaultSite(c.Request(), c.Path())
		data.CurrentUser = user

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		userUUID, err := uuid.Parse(user.ID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Authenticated user ID", slog.String("userID", userUUID.String()))

		now := time.Now()

		daysSinceMonday := (int(now.Weekday()) + 6) % 7
		weekStart := time.Date(
			now.Year(),
			now.Month(),
			now.Day()-daysSinceMonday,
			0, 0, 0, 0,
			now.Location(),
		)

		counts, err := repo.GetDashboardCounts(ctx, repository.GetDashboardCountsParams{
			WeekStart: pgtype.Timestamptz{Time: weekStart, Valid: true},
			ViewerID:  &userUUID,
		})

		viewCounts := views.DashboardCounts{
			ActivePatients:      int(counts.ActivePatients),
			NewPatientsThisWeek: int(counts.NewPatientsWeek),
			AwaitingReview:      int(counts.AwaitingReview),
			InProgress:          int(counts.InProgress),
			InProgressRemote:    int(counts.InProgressRemote),
			Completed7d:         int(counts.Completed7d),
			Completed7dDeltaPct: int(counts.CompletedPrev7d),
		}

		attentionItems, err := repo.ListAttentionItems(ctx, repository.ListAttentionItemsParams{
			RowLimit: 10,
			ViewerID: &userUUID,
		})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		viewAttentionItems := helpers.MapSlice(attentionItems, func(i *repository.ListAttentionItemsRow) dashboard.AttentionItemProps {
			isRemote := enums.GetAssignmentModeFromString(string(i.Mode)) == enums.AssignmentModes.REMOTE
			examID := i.ExaminationID.String()

			var note string
			var action dashboard.AttentionAction

			switch {
			case i.Kind == "REVIEW":
				note = string(i.Mode)
				action = dashboard.AttentionAction{Kind: dashboard.ActionReview, Href: "/assignments/" + i.ItemID.String()}

			case !isRemote: // overdue, in clinic
				note = "In Clinic"
				action = dashboard.AttentionAction{Kind: dashboard.ActionResume, Href: "/examinating/" + examID}

			case i.LinkUsable: // overdue, remote, link still valid -> nudge
				note = string(i.Mode)
				action = dashboard.AttentionAction{Kind: dashboard.ActionRemind, Href: "/remind/" + examID}

			default: // overdue, remote, link expired/used/revoked -> issue a new one
				note = string(i.Mode)
				action = dashboard.AttentionAction{Kind: dashboard.ActionResend, Href: "/resend/" + examID}
			}

			return dashboard.AttentionItemProps{
				ID:          i.ItemID.String(),
				PatientName: i.PatientName,
				TestCode:    i.TestCodes,
				Status:      enums.GetAssignmentStatusFromString(string(i.Status)),
				Overdue:     i.Overdue,
				Note:        note,
				Action:      action,
			}
		})

		weeklyCompletition, err := repo.GetWeeklyCompletion(ctx, repository.GetWeeklyCompletionParams{
			Tz:        "America/Toronto",
			WeekStart: pgtype.Timestamptz{Time: weekStart, Valid: true},
			ViewerID:  &userUUID,
		})

		slog.Debug("weeklyCompletition", "weeklyCompletition", weeklyCompletition, "err", err)

		viewWeeklyCompletition :=
			dashboard.WeeklyCompletionProps{
				Title: "Completion this week",
				Days: helpers.MapSlice(weeklyCompletition, func(i *repository.GetWeeklyCompletionRow) dashboard.DailyCompletion {
					return dashboard.DailyCompletion{
						Label:     time.Now().AddDate(0, 0, int(i.DayOffset)).Format("Mon"),
						Completed: int(i.Completed),
						Assigned:  int(i.Assigned),
					}
				}),
			}

		slog.Debug("viewWeeklyCompletition", "viewWeeklyCompletition", viewWeeklyCompletition)

		props := views.DashboardPageProps{
			Counts:    viewCounts,
			Attention: viewAttentionItems,
			Weekly:    viewWeeklyCompletition,
		}

		html := helpers.MustRenderHTML(views.DashboardPage(data, props))

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

		data := config.GetDefaultSite(c.Request(), c.Path())
		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		slog.Debug("Canonical", slog.String("canonical", data.Metatags.Canonical))

		html := helpers.MustRenderHTML(views.Index(data))

		return c.Blob(http.StatusOK, "text/html", html)

	}
}
