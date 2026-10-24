package controllers

import (
	"encoding/json"
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
	"github.com/Francesco99975/shorehamex2/views/assignments"
	"github.com/Francesco99975/shorehamex2/views/components"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

func Examinations() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("examinations", "Examinations", c.Request().Header.Get("X-Request-ID"))
		var status, mode *string
		var page int32

		isHX := c.Request().Header.Get("HX-Request") == "true"
		isBoosted := c.Request().Header.Get("HX-Boosted") == "true"

		// A boosted nav-in is still "a full page", just swapped by htmx.
		// Only requests that are HTMX but NOT boosted are your internal
		// filter/pagination fragment swaps.
		isFragment := isHX && !isBoosted

		if c.QueryParam("status") != "" {
			status = new(c.QueryParam("status"))
		}
		if c.QueryParam("mode") != "" {
			mode = new(c.QueryParam("mode"))
		}
		if c.QueryParam("page") != "" {
			rawPage, err := strconv.ParseInt(c.QueryParam("page"), 10, 32)
			if err != nil {
				page = 1
			} else {
				page = int32(rawPage)
			}
		} else {
			page = 1
		}
		// instruments := c.QueryParam("instruments")

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

		userUUID, err := uuid.Parse(user.ID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Authenticated user ID", slog.String("userID", userUUID.String()))

		examinations, err := repo.ListExaminations(c.Request().Context(), repository.ListExaminationsParams{
			PatientID:          nil,
			ViewerID:           &userUUID,
			OnlyOverdue:        nil,
			OnlyAwaitingReview: nil,
			Status:             status,
			Mode:               mode,
			RowOffset:          page - 1,
			RowLimit:           int32(boot.Environment.PaginationWindow),
		})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Fetched examinations", slog.Int("count", len(examinations)))

		examinationItems := helpers.MapSlice(examinations, func(a *repository.ListExaminationsRow) components.ExaminationItemProps {
			var remoteLink string
			if a.RemoteToken != nil {
				key, err := helpers.ParseBase64Key(boot.Environment.MetricSecret)
				if err != nil {
					slog.Error("Failed to parse metric secret", slog.String("error", err.Error()))
					key = nil
				}

				decryptedToken, err := helpers.Decrypt(string(a.RemoteToken), key)
				if err != nil {
					slog.Error("Failed to decrypt remote token", slog.String("error", err.Error()))
					decryptedToken = nil
				}

				if decryptedToken != nil {
					remoteLink = boot.Environment.URL + "/examinating/" + a.ID.String() + "?token=" + string(decryptedToken)
				} else {
					slog.Warn("Failed to decrypt remote token on it being nil")
				}
			}

			var progress []components.ExaminationProgress
			if a.AssignmentChips != nil {
				raw, err := json.Marshal(a.AssignmentChips)
				if err != nil {
					panic(err)
				}
				if err := json.Unmarshal(raw, &progress); err != nil {
					panic(err)
				}
			}

			return components.ExaminationItemProps{
				ID:          a.ID.String(),
				Initials:    a.PatientName[0:2],
				PatientName: a.PatientName,
				Remote:      a.Mode == repository.AssignmentModeREMOTE,
				Status:      a.Status,
				Overdue:     a.DueAt.Time.Before(time.Now()) && a.Status != repository.ExaminationStatusCOMPLETED,
				Due:         a.DueAt.Time.Format("January 2, 2006"),
				Progress:    progress,
				Assigned:    a.CreatedAt.Time.Format("January 2, 2006"),
				RemoteLink:  remoteLink,
			}
		})

		var html []byte
		if isFragment {
			html = helpers.MustRenderHTML(views.ExaminationsList(examinationItems, data.CSRF))
			slog.Debug("examinations list", "count", len(examinationItems))
			html = append(html, helpers.MustRenderHTML(components.EmptyState(len(examinationItems) == 0, true, "No examinations found matching your search criteria."))...)
		} else {
			html = helpers.MustRenderHTML(views.Examinations(data, examinationItems, len(examinations)))
		}

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func Assign() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("assigning tests", "Assign", c.Request().Header.Get("X-Request-ID"))

		var payload models.AssignPayload
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

		patientUUID, err := uuid.Parse(payload.PatientID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid patient id: %v", err))
		}

		slog.Debug("Assign payload", slog.Any("payload", payload))

		// 1. Create the examination (delivery unit)
		var examination *repository.CreateExaminationRow
		_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("examinations_pkey"), func(id uuid.UUID) error {
			examination, err = repo.CreateExamination(ctx, repository.CreateExaminationParams{
				ID:         id,
				PatientID:  patientUUID,
				Mode:       payload.Mode,
				Status:     repository.ExaminationStatusASSIGNED,
				AssignedBy: &auserUUID,
				DueAt: pgtype.Timestamptz{
					Time:  payload.DateToTime(),
					Valid: true,
				},
			})
			return err
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("create examination: %w", err))
		}

		// 2. Create one assignment per instrument, ordered by sequence
		for i, instrument := range payload.Instruments {
			var assignment *repository.Assignment
			_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("assignments_pkey"), func(id uuid.UUID) error {
				assignment, err = repo.CreateAssignment(ctx, repository.CreateAssignmentParams{
					ID:            id,
					ExaminationID: examination.ID,
					TestCode:      instrument,
					Sequence:      int16(i + 1), // 1-based
					// status defaults to ASSIGNED in the query; patient_id + is_developer_data come from the examination
					ViewerID: &auserUUID,
				})
				return err
			})
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("create assignment %s: %w", instrument, err))
			}

			// Create the empty results envelope (same as before)
			_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("assignment_results_pkey"), func(id uuid.UUID) error {
				_, err = repo.CreateAssignmentResult(ctx, repository.CreateAssignmentResultParams{
					ID:           id,
					AssignmentID: assignment.ID,
					ViewerID:     auserUUID,
				})
				return err
			})
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("create assignment result: %w", err))
			}
		}

		// 3. Remote → one token for the whole examination
		if payload.Mode == repository.AssignmentModeREMOTE {

			token, err := boot.Slugerr.New()
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			remoteLink := boot.Environment.URL + "/examinating/" + examination.ID.String() + "?token=" + token.String()

			helpers.ResendAssessmentInviatationTemplate(*examination.PatientEmail, examination.PatientName, remoteLink, examination.DueAt.Time.Format("2006-01-02 15:04:05"), strings.Join(payload.Instruments, ","), "casamedica@gmail.com")

			tokenHash, err := helpers.HashPassword(token.String())
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			key, err := helpers.ParseBase64Key(boot.Environment.MetricSecret)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			encryptedToken, err := helpers.Encrypt([]byte(token.String()), key)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, err)
			}

			_, err = helpers.GenerateProofUUIDV4(database.IsPKCollision("remote_access_tokens_pkey"), func(id uuid.UUID) error {
				_, err = repo.SendRemoteAccessToken(ctx, repository.SendRemoteAccessTokenParams{
					ID:             id,
					ExaminationID:  examination.ID,
					TokenHash:      tokenHash,
					TokenEncrypted: []byte(encryptedToken),
					SentToEmail:    *examination.PatientEmail,
					ExpiresAt:      examination.DueAt,
					ViewerID:       &auserUUID,
				})
				return err
			})
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("send remote token: %w", err))
			}

			newExamination, err := repo.GetExaminationWithPatient(ctx, repository.GetExaminationWithPatientParams{
				ID:       examination.ID,
				ViewerID: &auserUUID,
			})
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("get examination: %w", err))
			}

			var progress []components.ExaminationProgress
			if newExamination.AssignmentChips != nil {
				raw, err := json.Marshal(newExamination.AssignmentChips)
				if err != nil {
					panic(err)
				}
				if err := json.Unmarshal(raw, &progress); err != nil {
					panic(err)
				}
			}

			props := components.ExaminationItemProps{
				ID:          newExamination.ID.String(),
				Initials:    newExamination.PatientName[0:2],
				PatientName: newExamination.PatientName,
				Remote:      newExamination.Mode == repository.AssignmentModeREMOTE,
				Status:      newExamination.Status,
				Overdue:     newExamination.DueAt.Time.Before(time.Now()) && newExamination.Status != repository.ExaminationStatusCOMPLETED,
				Due:         newExamination.DueAt.Time.Format("January 2, 2006"),
				Progress:    progress,
				Assigned:    newExamination.CreatedAt.Time.Format("January 2, 2006"),
				RemoteLink:  remoteLink,
			}

			tools.SetToastTrigger(c.Response(), enums.SuccessToast, "Remote examination sent successfully")
			csrf := c.Get("csrf").(string)
			html := helpers.MustRenderHTML(components.ExaminationItem(props, csrf))
			html = append(html, helpers.MustRenderHTML(components.EmptyState(false, true, ""))...)
			return c.Blob(http.StatusCreated, "text/html", html)
		}

		c.Response().Header().Set("HX-Redirect", "/examinating/"+examination.ID.String())
		return c.NoContent(http.StatusSeeOther)
	}
}

func DeleteExamination() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("deleting examination", "DeleteExamination", c.Request().Header.Get("X-Request-ID"))
		examinationID := c.Param("id")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
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

		exminationUUID, err := uuid.Parse(examinationID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		remaining, err := repo.DeleteExamination(ctx, repository.DeleteExaminationParams{
			ID:       exminationUUID,
			ViewerID: &auserUUID,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		slog.Debug("delete examination", "remaining", remaining)

		html := helpers.MustRenderHTML(components.EmptyState(remaining == 0, true, "No examinations yet..."))
		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func StartExamination() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("starting examination", "StartExamination", c.Request().Header.Get("X-Request-ID"))
		examinationId := c.Param("id")
		token := c.QueryParam("token")

		slog.Debug("start examination", "examinationId", examinationId, "token", token)

		ctx := c.Request().Context()
		repo := repository.New(database.Pool())

		if examinationId == "" {
			return herr.HandleEchoPage(http.StatusBadRequest, fmt.Errorf("examination id not found"))
		}
		examinationUUID, err := uuid.Parse(examinationId)
		if err != nil {
			return herr.HandleEchoPage(http.StatusBadRequest, fmt.Errorf("invalid examination id: %w", err))
		}

		examination, err := repo.GetExamination(ctx, repository.GetExaminationParams{
			ID:       examinationUUID,
			ViewerID: nil,
		})
		if err != nil {
			return herr.HandleEchoPage(http.StatusNotFound, fmt.Errorf("examination not found: %w", err))
		}

		if examination.Mode == repository.AssignmentModeREMOTE {
			tokenData, err := repo.GetActiveRemoteAccessToken(ctx, repository.GetActiveRemoteAccessTokenParams{
				ExaminationID: examinationUUID,
				ViewerID:      nil,
			})
			if err != nil {
				return herr.HandleEchoPage(http.StatusUnauthorized, fmt.Errorf("no active remote access token: %w", err))
			}

			if !helpers.CheckPasswordHash(token, tokenData.TokenHash) {
				return herr.HandleEchoPage(http.StatusUnauthorized, fmt.Errorf("invalid token"))
			}
		}

		assignment, err := repo.GetNextAssignment(ctx, repository.GetNextAssignmentParams{
			ExaminationID: &examinationUUID,
			ViewerID:      nil,
		})
		if err != nil || assignment == nil {
			_, err = repo.MarkExaminationCompleted(ctx, repository.MarkExaminationCompletedParams{
				ID:       examinationUUID,
				ViewerID: nil,
			})
			if err != nil {
				return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("could not mark examination completed: %w", err))
			}
			err = repo.RevokeActiveRemoteAccessToken(ctx, repository.RevokeActiveRemoteAccessTokenParams{
				ExaminationID: examinationUUID,
			})
			if err != nil {
				return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("could not revoke active remote access token: %w", err))
			}

			if examination.Mode == repository.AssignmentModeREMOTE {
				return c.Redirect(http.StatusSeeOther, "/success")
			} else {
				c.Response().Header().Set("HX-Redirect", "/examinations")
				return c.NoContent(http.StatusSeeOther)
			}
		}

		tests, err := repo.ListTestDefinitions(c.Request().Context(), repository.ListTestDefinitionsParams{
			ActiveOnly: nil,
			Domain:     nil,
			RowOffset:  0,
			RowLimit:   50,
		})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("list test definitions: %w", err))
		}

		data := config.GetDefaultSite(c.Request(), c.Path())

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		if assignmentTest, ok := helpers.FirstWhere(tests, func(t *repository.ListTestDefinitionsRow) bool {
			return t.Code == assignment.TestCode
		}); !ok {
			return herr.HandleEchoPage(http.StatusNotFound, fmt.Errorf("test not found"))
		} else {
			switch assignmentTest.Code {
			case enums.TestCodes.ASQ.String():
				definition, err := models.LoadAsqDefinition(assignmentTest.SourcePath)
				if err != nil {
					return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("load asq definition: %w", err))
				}

				html := helpers.MustRenderHTML(assignments.Asq(data, definition.Content.Questions, definition.Content.Multiq, fmt.Sprintf("/examinating/asq/%s?token=%s", assignment.ID.String(), token)))

				return c.Blob(http.StatusOK, "text/html", html)

			case enums.TestCodes.BAI.String():
				definition, err := models.LoadBaiDefinition(assignmentTest.SourcePath)
				if err != nil {
					return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("load bai definition: %w", err))
				}
				html := helpers.MustRenderHTML(assignments.Bai(data, definition.Content.Questions, fmt.Sprintf("/examinating/bai/%s?token=%s", assignment.ID.String(), token)))
				return c.Blob(http.StatusOK, "text/html", html)
			case enums.TestCodes.BDI.String():
				definition, err := models.LoadBdiDefinition(assignmentTest.SourcePath)
				if err != nil {
					return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("load bdi definition: %w", err))
				}
				html := helpers.MustRenderHTML(assignments.Bdi(data, definition.Content.Questions, fmt.Sprintf("/examinating/bdi/%s?token=%s", assignment.ID.String(), token)))
				return c.Blob(http.StatusOK, "text/html", html)
			case enums.TestCodes.P3.String():
				definition, err := models.LoadP3Definition(assignmentTest.SourcePath)
				if err != nil {
					return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("load p3 definition: %w", err))
				}
				html := helpers.MustRenderHTML(assignments.P3(data, definition.Content.Questions, fmt.Sprintf("/examinating/p3/%s?token=%s", assignment.ID.String(), token)))
				return c.Blob(http.StatusOK, "text/html", html)
			case enums.TestCodes.DASS42.String():
				definition, err := models.LoadDass42Definition(assignmentTest.SourcePath)
				if err != nil {
					return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("load dass42 definition: %w", err))
				}
				html := helpers.MustRenderHTML(assignments.Dass42(data, definition.Content.Questions, fmt.Sprintf("/examinating/dass42/%s?token=%s", assignment.ID.String(), token)))
				return c.Blob(http.StatusOK, "text/html", html)
			case enums.TestCodes.BDI2.String():
				definition, err := models.LoadBDI2Definition(assignmentTest.SourcePath)
				if err != nil {
					return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("load BDI-II definition: %w", err))
				}
				html := helpers.MustRenderHTML(assignments.BDI2(data, definition.Content.Topics, fmt.Sprintf("/examinating/bdi2/%s?token=%s", assignment.ID.String(), token)))
				return c.Blob(http.StatusOK, "text/html", html)
			case enums.TestCodes.PSTD.String():
				definition, err := models.LoadPTSDDefinition(assignmentTest.SourcePath)
				if err != nil {
					return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("load ptsd definition: %w", err))
				}
				html := helpers.MustRenderHTML(assignments.PTSD(data, definition.Content.Questions, fmt.Sprintf("/examinating/ptsd/%s?token=%s", assignment.ID.String(), token)))
				return c.Blob(http.StatusOK, "text/html", html)
			case enums.TestCodes.MMPI2.String():
				definition, err := models.LoadMMPIDefinition(assignmentTest.SourcePath)
				if err != nil {
					return herr.HandleEchoPage(http.StatusInternalServerError, fmt.Errorf("load mmpi definition: %w", err))
				}

				questionsPerPage := 25
				startIndex := int(assignment.Page-1) * questionsPerPage
				endIndex := startIndex + questionsPerPage

				if startIndex < 0 || startIndex >= len(definition.Content.Questions) {
					return echo.NewHTTPError(404, "Page not found", http.StatusNotFound)
				}

				if endIndex > len(definition.Content.Questions) {
					endIndex = len(definition.Content.Questions)
				}

				paginatedQuestions := definition.Content.Questions[startIndex:endIndex]
				html := helpers.MustRenderHTML(assignments.MMPI(data, paginatedQuestions, int(assignment.Page), fmt.Sprintf("/examinating/mmpi2/%s?token=%s", assignment.ID.String(), token)))
				return c.Blob(http.StatusOK, "text/html", html)
			default:
				return herr.HandleEchoPage(http.StatusNotFound, fmt.Errorf("test not found"))
			}
		}

	}
}
