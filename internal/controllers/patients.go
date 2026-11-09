package controllers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/auth"
	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/Francesco99975/shorehamex2/internal/database"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/internal/httperr"
	"github.com/Francesco99975/shorehamex2/internal/middlewares"
	"github.com/Francesco99975/shorehamex2/internal/models"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/Francesco99975/shorehamex2/views"
	"github.com/Francesco99975/shorehamex2/views/components"
	"github.com/Francesco99975/shorehamex2/views/ui"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

type patientFilter struct {
	Query string
	Page  int
}

func parsePatientFilter(c echo.Context) patientFilter {
	f := patientFilter{Page: 1}

	q := strings.TrimSpace(c.QueryParam("q"))
	if r := []rune(q); len(r) > 100 {
		q = string(r[:100])
	}
	f.Query = q

	if p, err := strconv.Atoi(c.QueryParam("page")); err == nil && p > 1 {
		f.Page = p
	}
	return f
}

func (f patientFilter) href(page int) string {
	v := url.Values{}
	if f.Query != "" {
		v.Set("q", f.Query)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	if enc := v.Encode(); enc != "" {
		return "/patients?" + enc
	}
	return "/patients"
}

// search returns the LIKE-escaped term for sqlc, or nil for "no search".
func (f patientFilter) search() *string {
	if f.Query == "" {
		return nil
	}
	s := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.Query)
	return &s
}

func deref[T any](p *T) (v T) {
	if p != nil {
		v = *p
	}
	return v
}

func patientInitials(name string) string {
	r := []rune(strings.TrimSpace(name))
	if len(r) > 2 {
		r = r[:2]
	}
	return string(r)
}

// patientItemProps is the single Patient -> card mapping (nil-safe).
func patientItemProps(p *repository.Patient) components.PatientItemProps {
	var status *enums.AssignmentStatus
	if p.ComputedStatus != nil {
		s := enums.GetAssignmentStatusFromString(string(*p.ComputedStatus))
		status = &s
	}

	lastActivity := "Never"
	if p.LastActivityAt.Valid {
		lastActivity = p.LastActivityAt.Time.Format("Mon, 02 Jan 2006")
	}

	sex := ""
	if p.Sex != nil {
		sex = string(*p.Sex)
	}

	return components.PatientItemProps{
		MRN:              p.Mrn,
		Initials:         patientInitials(p.FullName),
		Name:             p.FullName,
		Email:            deref(p.Email),
		Phone:            deref(p.Phone),
		DOB:              p.DateOfBirth.Time.Format("2006-01-02"),
		Sex:              enums.GetSexFromString(sex),
		LastActivity:     lastActivity,
		AssignmentStatus: status,
	}
}

func Patients() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("patients", "Patients", c.Request().Header.Get("X-Request-ID"))
		req := c.Request()

		isFragment := req.Header.Get("HX-Request") == "true" &&
			req.Header.Get("HX-Boosted") != "true" &&
			req.Header.Get("HX-History-Restore-Request") != "true"

		f := parsePatientFilter(c)
		perPage := boot.Environment.PaginationWindow
		repo := repository.New(database.Pool())

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

		userID, err := uuid.Parse(user.ID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Authenticated user ID", slog.String("userID", userID.String()))

		ctx := req.Context()

		total, err := repo.CountPatientsFiltered(ctx, repository.CountPatientsFilteredParams{
			ViewerID: &userID,
			Search:   f.search(),
		})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		totalPages := max(1, (int(total)+perPage-1)/perPage)
		f.Page = min(f.Page, totalPages)

		if canonical := f.href(f.Page); !isFragment && req.URL.RequestURI() != canonical {
			return c.Redirect(http.StatusFound, canonical)
		}

		rawPatients, err := repo.ListPatients(ctx, repository.ListPatientsParams{
			ViewerID:  &userID,
			Search:    f.search(),
			RowOffset: int32((f.Page - 1) * perPage),
			RowLimit:  int32(perPage),
		})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		vm := views.PatientsViewData{
			Items:   helpers.MapSlice(rawPatients, patientItemProps),
			Total:   int(total),
			Page:    f.Page,
			PerPage: perPage,
			Query:   f.Query,
			Href:    f.href,
		}

		c.Response().Header().Add("Vary", "HX-Request")

		var html []byte
		if isFragment {
			c.Response().Header().Set("HX-Push-Url", f.href(f.Page))
			html = helpers.MustRenderHTML(views.PatientsResults(vm, data.CSRF))
		} else {
			html = helpers.MustRenderHTML(views.Patients(data, vm))
		}
		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func PickPatient() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("picking patient", "PickPatient", c.Request().Header.Get("X-Request-ID"))
		search := c.QueryParam("patient")

		ctx := c.Request().Context()

		repo := repository.New(database.Pool())

		auser, err := auth.GetActiveSession(c.Request(), repo)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to get active session: %v", err))
		}
		if auser == nil {
			return c.Redirect(http.StatusSeeOther, "/auth")
		}

		auserUUID, err := uuid.Parse(auser.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to parse user id: %v", err))
		}

		rawPatients, err := repo.PickPatientSearch(ctx, repository.PickPatientSearchParams{
			ViewerID: &auserUUID,
			Query:    search,
			RowLimit: int32(boot.Environment.PaginationWindow),
			Page:     1,
		})

		slog.Debug("picking patients", "search", search, "count", len(rawPatients))

		patients := helpers.MapSlice(rawPatients, func(rawPatient *repository.PickPatientSearchRow) ui.SearchResultsItemProps {
			return ui.SearchResultsItemProps{
				Name: rawPatient.FullName,
				ID:   rawPatient.ID.String(),
			}
		})

		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get users: %v", err))
		}

		html := helpers.MustRenderHTML(ui.SearchResults(patients))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func AddPatient() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("adding patient", "AddPatient", c.Request().Header.Get("X-Request-ID"))
		var payload models.AddPatientPayload
		if err := c.Bind(&payload); err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		if err := payload.Validate(); err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		repo := repository.New(tx)
		defer database.HandleTransaction(ctx, tx, &err)

		auserID, ok := ctx.Value(middlewares.UserKey).(string)
		if !ok {
			return herr.Handle(c.Response(), http.StatusInternalServerError, errors.New("user id not found"))
		}

		auserUUID, err := uuid.Parse(auserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		newPatientID, newPatientMRN, err := helpers.GenerateProofUUIDv7WithMRN(database.NewClassifier("patients_pkey", "patients_mrn_key"), func(id uuid.UUID, mrn string) error {
			var err error

			_, err = repo.CreatePatient(ctx, repository.CreatePatientParams{
				ID:       id,
				Mrn:      mrn,
				FullName: payload.FullName,
				Email:    &payload.Email,
				Phone:    &payload.Phone,
				DateOfBirth: pgtype.Date{
					Time:  payload.DobToTime(),
					Valid: true,
				},
				Sex:       &payload.Sex,
				CreatedBy: &auserUUID,
			})

			if err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		slog.Info("patient created", "id", newPatientID, "mrn", newPatientMRN)

		c.Response().Header().Set("HX-Trigger", "patients-changed")
		return c.NoContent(http.StatusNoContent)
	}
}

func UpdatePatient() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("updating patient", "UpdatePatient", c.Request().Header.Get("X-Request-ID"))

		mrn := c.Param("id")
		if mrn == "" {
			return herr.Handle(c.Response(), http.StatusBadRequest, errors.New("patient unidentified"))
		}

		var payload models.AddPatientPayload
		if err := c.Bind(&payload); err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		if err := payload.Validate(); err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		repo := repository.New(tx)
		defer database.HandleTransaction(ctx, tx, &err)

		auserID, ok := ctx.Value(middlewares.UserKey).(string)
		if !ok {
			return herr.Handle(c.Response(), http.StatusInternalServerError, errors.New("user id not found"))
		}

		auserUUID, err := uuid.Parse(auserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		_, err = repo.UpdatePatientDetails(ctx, repository.UpdatePatientDetailsParams{
			Mrn:      mrn,
			FullName: payload.FullName,
			Email:    &payload.Email,
			Phone:    &payload.Phone,
			DateOfBirth: pgtype.Date{
				Time:  payload.DobToTime(),
				Valid: true,
			},
			Sex:      &payload.Sex,
			ViewerID: &auserUUID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Not found, OR outside the viewer's partition. Same answer for both.
			return herr.Handle(c.Response(), http.StatusNotFound, errors.New("patient not found"))
		}
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		c.Response().Header().Set("HX-Trigger", "patients-changed")
		return c.NoContent(http.StatusNoContent)
	}
}

func DeletePatient() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("deleting patient", "DeletePatient", c.Request().Header.Get("X-Request-ID"))

		mrn := c.Param("id")
		if mrn == "" {
			return herr.Handle(c.Response(), http.StatusBadRequest, errors.New("patient unidentified"))
		}

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		repo := repository.New(tx)
		defer database.HandleTransaction(ctx, tx, &err)

		auserID, ok := ctx.Value(middlewares.UserKey).(string)
		if !ok {
			return herr.Handle(c.Response(), http.StatusInternalServerError, errors.New("user id not found"))
		}

		auserUUID, err := uuid.Parse(auserID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		rows, err := repo.DeletePatient(ctx, repository.DeletePatientParams{
			Mrn:      mrn,
			ViewerID: &auserUUID,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		if rows == 0 {
			return herr.Handle(c.Response(), http.StatusNotFound, errors.New("patient not found"))
		}

		c.Response().Header().Set("HX-Trigger", "patients-changed")
		return c.NoContent(http.StatusOK)
	}
}
