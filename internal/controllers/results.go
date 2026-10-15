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
	"github.com/Francesco99975/shorehamex2/internal/models"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/Francesco99975/shorehamex2/views"
	"github.com/Francesco99975/shorehamex2/views/results"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func ExaminationDetail() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("examination_detail", "ExaminationDetail", c.Request().Header.Get("X-Request-ID"))
		id := c.Param("id")
		if id == "" {
			return herr.HandleEchoPage(http.StatusBadRequest, nil)
		}

		examinationUUID, err := uuid.Parse(id)
		if err != nil {
			return herr.HandleEchoPage(http.StatusBadRequest, err)
		}

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

		userUUID, err := uuid.Parse(user.ID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Authenticated user ID", slog.String("userID", userUUID.String()))

		examinationDetails, err := repo.GetExaminationDetail(c.Request().Context(), examinationUUID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Fetched examination details", slog.String("examinationID", examinationDetails.Patient.Mrn))

		patientIdentityProps := results.PatientIdentityProps{
			ID:          examinationDetails.Patient.ID.String(),
			MRN:         examinationDetails.Patient.Mrn,
			FullName:    examinationDetails.Patient.FullName,
			Email:       *examinationDetails.Patient.Email,
			Phone:       *examinationDetails.Patient.Phone,
			DateOfBirth: examinationDetails.Patient.DateOfBirth.Time,
			Sex:         enums.GetSexFromString(string(*examinationDetails.Patient.Sex)),
			Now:         time.Now(),
		}

		examinationsAssignments, err := repo.ListExaminationAssignments(c.Request().Context(), &examinationUUID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		var assignments []results.AssignmentRowProps = make([]results.AssignmentRowProps, 0, len(examinationsAssignments))
		var steps []results.ProgressStep = make([]results.ProgressStep, 0, len(examinationsAssignments))
		for _, assignment := range examinationsAssignments {
			assignments = append(assignments, results.AssignmentRowProps{
				ID:          assignment.ID.String(),
				Sequence:    int(assignment.Sequence),
				Page:        int(assignment.Page),
				TestCode:    assignment.TestCode,
				Status:      enums.GetAssignmentStatusFromString(string(assignment.Status)),
				StartedAt:   &assignment.StartedAt.Time,
				CompletedAt: &assignment.CompletedAt.Time,
				ReviewedAt:  &assignment.ReviewedAt.Time,
			})
			steps = append(steps, results.ProgressStep{
				Sequence: int(assignment.Sequence),
				TestCode: assignment.TestCode,
				Done:     assignment.Status == repository.AssignmentStatusCOMPLETED || assignment.Status == repository.AssignmentStatusREADYFORREVIEW,
			})
		}

		metaProps := results.ExaminationMetaProps{
			Mode:        enums.AssignmentMode(examinationDetails.Examination.Mode),
			Status:      examinationDetails.Examination.Status,
			Overdue:     examinationDetails.Examination.DueAt.Time.Before(time.Now()),
			AssignedAt:  examinationDetails.Patient.CreatedAt.Time,
			DueAt:       &examinationDetails.Examination.DueAt.Time,
			StartedAt:   &examinationDetails.Examination.StartedAt.Time,
			CompletedAt: &examinationDetails.Examination.CompletedAt.Time,
			ReviewedAt:  &examinationDetails.Examination.ReviewedAt.Time,
			Steps:       steps,
		}

		props := views.ExaminationDetailPageProps{
			ExaminationID: examinationUUID.URN(),
			Patient:       patientIdentityProps,
			Examination:   metaProps,
			Assignments:   assignments,
		}

		html := helpers.MustRenderHTML(views.ExaminationDetailPage(data, props))

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func AssignmentDetail() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("assignment_detail", "AssignmentDetail", c.Request().Header.Get("X-Request-ID"))
		id := c.Param("id")
		if id == "" {
			return herr.HandleEchoPage(http.StatusBadRequest, nil)
		}

		assignmentUUID, err := uuid.Parse(id)
		if err != nil {
			return herr.HandleEchoPage(http.StatusBadRequest, err)
		}

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

		userUUID, err := uuid.Parse(user.ID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Authenticated user ID", slog.String("userID", userUUID.String()))

		assignmentDetails, err := repo.GetAssignmentDetail(c.Request().Context(), assignmentUUID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Fetched assignment details", slog.String("assignmentID", assignmentDetails.Assignment.ID.String()))

		patientIdentityProps := results.PatientIdentityProps{
			ID:          assignmentDetails.Patient.ID.String(),
			MRN:         assignmentDetails.Patient.Mrn,
			FullName:    assignmentDetails.Patient.FullName,
			Email:       *assignmentDetails.Patient.Email,
			Phone:       *assignmentDetails.Patient.Phone,
			DateOfBirth: assignmentDetails.Patient.DateOfBirth.Time,
			Sex:         enums.GetSexFromString(string(*assignmentDetails.Patient.Sex)),
			Now:         time.Now(),
		}

		headerProps := results.AssignmentHeaderProps{
			ID:       assignmentDetails.Assignment.ID.String(),
			Patient:  patientIdentityProps,
			TestCode: assignmentDetails.Assignment.TestCode,
			Status:   enums.GetAssignmentStatusFromString(string(assignmentDetails.Assignment.Status)),
			Page:     int(assignmentDetails.Assignment.Page),
			Timing: results.AssignmentTimingProps{
				StartedAt:       &assignmentDetails.Assignment.StartedAt.Time,
				CompletedAt:     &assignmentDetails.Assignment.CompletedAt.Time,
				DurationSeconds: new(int(*assignmentDetails.DurationSeconds)),
			},
		}

		props := views.AssignmentDetailPageProps{
			Header:      headerProps,
			Result:      nil,
			MMPIResults: nil,
			BackHref:    "/examinations/" + assignmentDetails.Assignment.ExaminationID.String(),
		}
		if assignmentDetails.Assignment.TestCode != "MMPI-2" {

			rawScores, err := results.DecodeRawScores(assignmentDetails.RawScores)
			if err != nil {
				return herr.HandleEchoPage(http.StatusInternalServerError, err)
			}
			responses, err := results.DecodeResponses(assignmentDetails.Responses)
			if err != nil {
				return herr.HandleEchoPage(http.StatusInternalServerError, err)
			}

			assignmentSumBandResults, err := repo.ListSumBandResults(c.Request().Context(), *assignmentDetails.ResultID)
			if err != nil {
				return herr.HandleEchoPage(http.StatusInternalServerError, err)
			}

			sumBands := make([]results.SumBand, 0, len(assignmentSumBandResults))
			for _, band := range assignmentSumBandResults {
				numeric, err := band.NumericScore.Int64Value()
				if err != nil {
					return err
				}

				value := int(numeric.Int64)
				sumBands = append(sumBands, results.SumBand{
					SubscaleCode:          band.SubscaleCode,
					HeadlineScore:         band.HeadlineScore,
					NumericScore:          value,
					ScaleLabel:            band.ScaleLabel,
					SeverityLabel:         band.SeverityLabel,
					InterpretationSummary: band.InterpretationSummary,
				})
			}

			assignmentResult := results.AssignmentResult{
				SumBands:  sumBands,
				RawScores: rawScores,
				Responses: responses,
			}

			props.Result = &assignmentResult
		} else {
			rawMmpiResults, err := repo.GetMMPIAssignment(c.Request().Context(), assignmentDetails.Assignment.ID)
			if err != nil {
				return herr.HandleEchoPage(http.StatusInternalServerError, err)
			}

			testDefinition, err := repo.GetTestDefinition(c.Request().Context(), assignmentDetails.Assignment.TestCode)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			mmpi2Definition, err := models.LoadMMPIDefinition(testDefinition.SourcePath)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			mmpiResults, err := models.CalculateMMPI(mmpi2Definition, rawMmpiResults.Answers, enums.GetSexFromString(string(*assignmentDetails.Patient.Sex)))
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			mmpiResults.Sex = string(*assignmentDetails.Patient.Sex)
			mmpiResults.Duration = int(*assignmentDetails.DurationSeconds)

			props.MMPIResults = &mmpiResults
		}

		html := helpers.MustRenderHTML(views.AssignmentDetailPage(data, props))

		return c.Blob(http.StatusOK, "text/html", html)

	}
}
