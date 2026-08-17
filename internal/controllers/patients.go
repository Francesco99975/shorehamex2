package controllers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

		rawPatients, err := repo.ListPatients(c.Request().Context(), repository.ListPatientsParams{
			ViewerID:  userID,
			RowOffset: 0,
			RowLimit:  25,
		})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		numberOfPatients, err := repo.CountActivePatients(c.Request().Context(), userID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		patients := helpers.MapSlice(rawPatients, func(rawPatient *repository.Patient) components.PatientItemProps {
			initials := rawPatient.FullName[0:2]
			var status *enums.AssignmentStatus
			if rawPatient.ComputedStatus != nil {
				status = new(enums.AssignmentStatus)
				*status = enums.GetAssignmentStatusFromString(string(*rawPatient.ComputedStatus))
			}

			var lastActivity string
			if rawPatient.LastActivityAt.Valid {
				lastActivity = rawPatient.LastActivityAt.Time.Format("Mon, 02 Jan 2006")
			} else {
				lastActivity = "Never"
			}

			return components.PatientItemProps{
				MRN:              rawPatient.Mrn,
				Initials:         initials,
				Name:             rawPatient.FullName,
				Email:            *rawPatient.Email,
				Phone:            *rawPatient.Phone,
				DOB:              rawPatient.DateOfBirth.Time.Format("2006-01-02"),
				Sex:              enums.GetSexFromString(string(*rawPatient.Sex)),
				LastActivity:     lastActivity,
				AssignmentStatus: status,
			}
		})

		total := int(numberOfPatients) - len(patients)

		html := helpers.MustRenderHTML(views.Patients(data, patients, total))

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func SearchPatients() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("searching patients", "SearchPatients", c.Request().Header.Get("X-Request-ID"))
		search := c.QueryParam("search_patients")

		slog.Debug("searching patients", "search", search)
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

		auserUUID, err := uuid.Parse(auser.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to parse user id: %v", err))
		}

		numberOfPatients, err := repo.CountActivePatients(ctx, auserUUID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to count active patients: %v", err))
		}

		rawPatients, err := repo.SearchPatients(ctx, repository.SearchPatientsParams{
			ViewerID: auserUUID,
			Query:    &search,
			RowLimit: int32(boot.Environment.PaginationWindow),
			Page:     page,
		})

		slog.Debug("searching patients", "search", search, "count", len(rawPatients))

		patients := helpers.MapSlice(rawPatients, func(rawPatient *repository.Patient) components.PatientItemProps {

			initials := rawPatient.FullName[0:2]
			var status *enums.AssignmentStatus
			if rawPatient.ComputedStatus != nil {
				status = new(enums.AssignmentStatus)
				*status = enums.GetAssignmentStatusFromString(string(*rawPatient.ComputedStatus))
			}

			var lastActivity string
			if rawPatient.LastActivityAt.Valid {
				lastActivity = rawPatient.LastActivityAt.Time.Format("Mon, 02 Jan 2006")
			} else {
				lastActivity = "Never"
			}

			return components.PatientItemProps{
				MRN:              rawPatient.Mrn,
				Initials:         initials,
				Name:             rawPatient.FullName,
				Email:            *rawPatient.Email,
				Phone:            *rawPatient.Phone,
				DOB:              rawPatient.DateOfBirth.Time.Format("2006-01-02"),
				Sex:              enums.GetSexFromString(string(*rawPatient.Sex)),
				LastActivity:     lastActivity,
				AssignmentStatus: status,
			}
		})

		total := int(numberOfPatients) - len(patients)

		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("unable to get users: %v", err))
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(views.PatientsList(patients, csrf))
		html = append(html, helpers.MustRenderHTML(views.PatientsPagination(int(total), page, boot.Environment.PaginationWindow, true))...)

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
		var newPatient *repository.Patient
		newPatientID, newPatientMRN, err := helpers.GenerateProofUUIDv7WithMRN(database.NewClassifier("patients_pkey", "patients_mrn_key"), func(id uuid.UUID, mrn string) error {
			var err error

			newPatient, err = repo.CreatePatient(ctx, repository.CreatePatientParams{
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

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.PatientItem(components.PatientItemProps{
			MRN:              newPatient.Mrn,
			Initials:         newPatient.FullName[0:2],
			Name:             newPatient.FullName,
			Email:            *newPatient.Email,
			Phone:            *newPatient.Phone,
			DOB:              newPatient.DateOfBirth.Time.Format("2006-01-02"),
			Sex:              enums.GetSexFromString(string(*newPatient.Sex)),
			LastActivity:     "Never",
			AssignmentStatus: nil,
		}, csrf))

		return c.Blob(http.StatusOK, "text/html", []byte(html))
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

		updatedPatient, err := repo.UpdatePatientDetails(ctx, repository.UpdatePatientDetailsParams{
			Mrn:      mrn,
			FullName: payload.FullName,
			Email:    &payload.Email,
			Phone:    &payload.Phone,
			DateOfBirth: pgtype.Date{
				Time:  payload.DobToTime(),
				Valid: true,
			},
			Sex:      &payload.Sex,
			ViewerID: auserUUID,
		})

		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		csrf := c.Get("csrf").(string)

		html := helpers.MustRenderHTML(components.PatientItem(components.PatientItemProps{
			MRN:              updatedPatient.Mrn,
			Initials:         updatedPatient.FullName[0:2],
			Name:             updatedPatient.FullName,
			Email:            *updatedPatient.Email,
			Phone:            *updatedPatient.Phone,
			DOB:              updatedPatient.DateOfBirth.Time.Format("2006-01-02"),
			Sex:              enums.GetSexFromString(string(*updatedPatient.Sex)),
			LastActivity:     "Never",
			AssignmentStatus: nil,
		}, csrf))

		return c.Blob(http.StatusOK, "text/html", []byte(html))
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

		if err := repo.DeletePatient(ctx, repository.DeletePatientParams{
			Mrn:      mrn,
			ViewerID: auserUUID,
		}); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		return c.NoContent(http.StatusOK)
	}
}
