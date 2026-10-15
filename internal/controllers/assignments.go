package controllers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/Francesco99975/shorehamex2/internal/database"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/internal/httperr"
	"github.com/Francesco99975/shorehamex2/internal/models"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/Francesco99975/shorehamex2/views/assignments"

	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
)

const MMPI_TEST_ANSWERS string = "TTFTFFTFFFFFFTFTFFFTTTFFFTFFTFTFFTFTTTTFTFFFFTTTFTTTTFTTTFTFFTTFTTTFTTTFTFF" +
	"TTFFFTTFFTFTTTFTTTFFFFTFFFFFFTFTTFTFFFFTFTTFTFFTFFFTTTTFFFFTFTFTFFFTFFFTFFT" +
	"FFFTTTFTTFTFTTFTTFFFTTFFTTFTTTTFFFTTFFFTFTFFTTFFTFFFFTFFTFTFFFFTTFTFTFFTFTF" +
	"TFFTFTTTFTTFTFFFFTFFFFFTTTFTFFTFTTTFFTTTTTTTFFFTFTTTTTFFFFTTFFFTFFFTFTFFFFF" +
	"TFTFFTFTTTTFFTFTFFFTFFFFTTFFFTFFFFFFTFTFTFFTFFTTTFFFTFFFTFFFFTFTTFFTFFTTFTF" +
	"TTTFTFFTTTTTFFTTFFTFFFFTTTFFTTFFTTFTTFFFFFTFTFFTTFFTFFFTTTTFFTFTTFTFFFTFFFT" +
	"TTFTTFFTTFTTTTTFTFTFFTTTTTFFTTFTTFTTTFTFTTFFFFTFTFTTFTTTFFTFTFFTTTTTTTFFTFF" +
	"TTFFTFTTFFFFFTFTFFFFFTFFTFFTTFFFTFFFTFTTTF"

func createBasicIndication(testName string, duration int, percentage string, gravity string) string {
	durationSec := duration / 1000
	durationMin := durationSec / 60
	durationSec = durationSec % 60
	return fmt.Sprintf("The patient scored %s on the %s in %d minutes and %d seconds. This result is in the %s range.", percentage, testName, durationMin, durationSec, gravity)
}

func ExaminationSuccess() echo.HandlerFunc {
	return func(c echo.Context) error {

		data := config.GetDefaultSite(c.Request())
		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		html := helpers.MustRenderHTML(assignments.ExaminationSuccess(data))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}

func AsqCalc() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("calucationg ASQ results", "AsqCalc", c.Request().Header.Get("X-Request-ID"))

		assignmentID := c.Param("id")
		assignmentUUID, err := uuid.Parse(assignmentID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		token := c.QueryParam("token")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)

		repo := repository.New(tx)

		assignment, err := repo.GetAssignment(ctx, repository.GetAssignmentParams{
			ID:       assignmentUUID,
			ViewerID: nil,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		testDefinition, err := repo.GetTestDefinition(ctx, assignment.TestCode)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		asqDefinition, err := models.LoadAsqDefinition(testDefinition.SourcePath)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		startUnix, err := strconv.ParseInt(c.FormValue("start"), 10, 64)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid start time: %w", err))
		}
		startTime := time.Unix(startUnix, 0)

		duration := int(time.Since(startTime).Milliseconds())

		var score int
		var responses map[string]string = make(map[string]string)

		for i := range asqDefinition.MaxScore {

			if i < 5 {
				asw, err := strconv.Atoi(c.FormValue(fmt.Sprintf("A%d", i)))
				if err != nil {
					return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("could not get answer: %w", err))
				}
				if asw == 1 {
					responses[asqDefinition.Content.Questions[i]] = "Yes"
					score++
				} else {
					responses[asqDefinition.Content.Questions[i]] = "No"
				}
			} else {
				asw := c.FormValue(fmt.Sprintf("MA%d", i-5))

				if asw == "on" {
					responses[asqDefinition.Content.Multiq[i-5]] = "Yes"
					score++
				} else {
					responses[asqDefinition.Content.Multiq[i-5]] = "No"
				}
			}
		}

		rawPercentage := float64(score) / float64(asqDefinition.MaxScore) * 100.0

		var gravity string
		if rawPercentage <= 30 {
			gravity = "normal"
		} else if rawPercentage >= 31 && rawPercentage <= 45 {
			gravity = "moderate"
		} else {
			gravity = "severe"
		}

		percentage := fmt.Sprintf("%.2f", rawPercentage) + "%"

		responsesRawJson, err := json.Marshal(responses)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		assignmentResult, err := repo.UpdateAssignmentResponses(ctx, repository.UpdateAssignmentResponsesParams{
			AssignmentID: assignment.ID,
			NewResponses: responsesRawJson,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		rawScores := make(map[string]any)
		rawScores["TOTAL"] = map[string]any{
			"gravity":    gravity,
			"percentage": percentage,
			"score":      score,
		}

		rawScoresJSON, err := json.Marshal(rawScores)
		if err != nil {
			return herr.Handle(
				c.Response(),
				http.StatusInternalServerError,
				fmt.Errorf("failed to marshal raw scores: %w", err),
			)
		}

		assignmentResult, err = repo.FinalizeAssignmentResultScoring(ctx, repository.FinalizeAssignmentResultScoringParams{
			AssignmentID:    assignmentUUID,
			RawScores:       rawScoresJSON,
			DurationSeconds: new(int32(duration / 1000)),
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		indication := createBasicIndication(asqDefinition.Name, duration, percentage, gravity)

		_, err = repo.CreateSumBandResult(ctx, repository.CreateSumBandResultParams{
			AssignmentResultID: assignmentResult.ID,
			HeadlineScore:      percentage,
			NumericScore: pgtype.Numeric{
				Int:   big.NewInt(int64(score)),
				Valid: true,
			},
			ScaleLabel:            fmt.Sprintf("%d out of %d", score, asqDefinition.MaxScore),
			SeverityLabel:         gravity,
			InterpretationSummary: indication,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to create sum band result: %w", err))
		}

		_, err = repo.MarkAssignmentReadyForReview(ctx, assignment.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark assignment ready for review: %w", err))
		}

		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/examinating/%s?token=%s", assignment.ExaminationID.String(), token))
	}
}

func BaiCalc() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("calculating BAI results", "BaiCalc", c.Request().Header.Get("X-Request-ID"))

		assignmentID := c.Param("id")
		assignmentUUID, err := uuid.Parse(assignmentID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		token := c.QueryParam("token")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)

		repo := repository.New(tx)

		assignment, err := repo.GetAssignment(ctx, repository.GetAssignmentParams{
			ID:       assignmentUUID,
			ViewerID: nil,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		testDefinition, err := repo.GetTestDefinition(ctx, assignment.TestCode)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		baiDefinition, err := models.LoadBaiDefinition(testDefinition.SourcePath)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		startUnix, err := strconv.ParseInt(c.FormValue("start"), 10, 64)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid start time: %w", err))
		}
		startTime := time.Unix(startUnix, 0)

		duration := int(time.Since(startTime).Milliseconds())

		var score int
		var responses map[string]string = make(map[string]string)

		for i := range 21 {

			asw, err := strconv.Atoi(c.FormValue(fmt.Sprintf("q%d", i)))
			if err != nil {
				return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid answer: %w", err))
			}

			switch asw {
			case 0:
				responses[baiDefinition.Content.Questions[i]] = "Not At All"
			case 1:
				responses[baiDefinition.Content.Questions[i]] = "Applied to me to some degree, or some of the time"
			case 2:
				responses[baiDefinition.Content.Questions[i]] = "Moderately - it wasn't pleasant at times"
			case 3:
				responses[baiDefinition.Content.Questions[i]] = "Severely - it bothered me a lot"
			default:
				responses[baiDefinition.Content.Questions[i]] = "Error response"
			}
			score += asw

		}

		rawPercentage := float64(score) / float64(baiDefinition.MaxScore) * 100.0

		var gravity string
		if rawPercentage <= 21 {
			gravity = "normal"
		} else if rawPercentage >= 22 && rawPercentage <= 35 {
			gravity = "moderate"
		} else {
			gravity = "severe"
		}

		percentage := fmt.Sprintf("%.2f", rawPercentage) + "%"

		responsesRawJson, err := json.Marshal(responses)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		assignmentResult, err := repo.UpdateAssignmentResponses(ctx, repository.UpdateAssignmentResponsesParams{
			AssignmentID: assignment.ID,
			NewResponses: responsesRawJson,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		rawScores := make(map[string]any)
		rawScores["TOTAL"] = map[string]any{
			"gravity":    gravity,
			"percentage": percentage,
			"score":      score,
		}

		rawScoresJSON, err := json.Marshal(rawScores)
		if err != nil {
			return herr.Handle(
				c.Response(),
				http.StatusInternalServerError,
				fmt.Errorf("failed to marshal raw scores: %w", err),
			)
		}

		assignmentResult, err = repo.FinalizeAssignmentResultScoring(ctx, repository.FinalizeAssignmentResultScoringParams{
			AssignmentID:    assignmentUUID,
			RawScores:       rawScoresJSON,
			DurationSeconds: new(int32(duration / 1000)),
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		indication := createBasicIndication(baiDefinition.Name, duration, percentage, gravity)

		_, err = repo.CreateSumBandResult(ctx, repository.CreateSumBandResultParams{
			AssignmentResultID: assignmentResult.ID,
			HeadlineScore:      percentage,
			NumericScore: pgtype.Numeric{
				Int:   big.NewInt(int64(score)),
				Valid: true,
			},
			ScaleLabel:            fmt.Sprintf("%d out of %d", score, baiDefinition.MaxScore),
			SeverityLabel:         gravity,
			InterpretationSummary: indication,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to create sum band result: %w", err))
		}

		_, err = repo.MarkAssignmentReadyForReview(ctx, assignment.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark assignment ready for review: %w", err))
		}

		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/examinating/%s?token=%s", assignment.ExaminationID.String(), token))
	}
}

func BdiCalc() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("calculating BDI results", "BdiCalc", c.Request().Header.Get("X-Request-ID"))

		assignmentID := c.Param("id")
		assignmentUUID, err := uuid.Parse(assignmentID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		token := c.QueryParam("token")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)

		repo := repository.New(tx)

		assignment, err := repo.GetAssignment(ctx, repository.GetAssignmentParams{
			ID:       assignmentUUID,
			ViewerID: nil,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		testDefinition, err := repo.GetTestDefinition(ctx, assignment.TestCode)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		bdiDefinition, err := models.LoadBdiDefinition(testDefinition.SourcePath)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		startUnix, err := strconv.ParseInt(c.FormValue("start"), 10, 64)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		startTime := time.Unix(startUnix, 0)

		duration := int(time.Since(startTime).Milliseconds())

		var score int
		var responses map[string]string = make(map[string]string)

		for i := range 11 {

			asw, err := strconv.Atoi(c.FormValue(fmt.Sprintf("%d", i)))
			if err != nil {
				return herr.Handle(c.Response(), http.StatusBadRequest, err)
			}

			responses[fmt.Sprintf("Q%d", i)] = bdiDefinition.Content.Questions[i][asw]

			score += asw

		}

		rawPercentage := float64(score) / float64(bdiDefinition.MaxScore) * 100.0

		var gravity string
		if rawPercentage <= 9 {
			gravity = "normal"
		} else if rawPercentage >= 10 && rawPercentage <= 18 {
			gravity = "moderate"
		} else {
			gravity = "severe"
		}

		percentage := fmt.Sprintf("%.2f", rawPercentage) + "%"

		responsesRawJson, err := json.Marshal(responses)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		assignmentResult, err := repo.UpdateAssignmentResponses(ctx, repository.UpdateAssignmentResponsesParams{
			AssignmentID: assignment.ID,
			NewResponses: responsesRawJson,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		rawScores := make(map[string]any)
		rawScores["TOTAL"] = map[string]any{
			"gravity":    gravity,
			"percentage": percentage,
			"score":      score,
		}

		rawScoresJSON, err := json.Marshal(rawScores)
		if err != nil {
			return herr.Handle(
				c.Response(),
				http.StatusInternalServerError,
				fmt.Errorf("failed to marshal raw scores: %w", err),
			)
		}

		assignmentResult, err = repo.FinalizeAssignmentResultScoring(ctx, repository.FinalizeAssignmentResultScoringParams{
			AssignmentID:    assignmentUUID,
			RawScores:       rawScoresJSON,
			DurationSeconds: new(int32(duration / 1000)),
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		indication := createBasicIndication(bdiDefinition.Name, duration, percentage, gravity)

		log.Debugf("Indication: %s", indication)

		_, err = repo.CreateSumBandResult(ctx, repository.CreateSumBandResultParams{
			AssignmentResultID: assignmentResult.ID,
			HeadlineScore:      percentage,
			NumericScore: pgtype.Numeric{
				Int:   big.NewInt(int64(score)),
				Valid: true,
			},
			ScaleLabel:            fmt.Sprintf("%d out of %d", score, bdiDefinition.MaxScore),
			SeverityLabel:         gravity,
			InterpretationSummary: indication,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to create sum band result: %w", err))
		}

		_, err = repo.MarkAssignmentReadyForReview(ctx, assignment.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark assignment ready for review: %w", err))
		}

		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/examinating/%s?token=%s", assignment.ExaminationID.String(), token))
	}
}

func P3Calc() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("calculating P3 results", "P3Calc", c.Request().Header.Get("X-Request-ID"))

		assignmentID := c.Param("id")
		assignmentUUID, err := uuid.Parse(assignmentID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		token := c.QueryParam("token")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)

		repo := repository.New(tx)

		assignment, err := repo.GetAssignment(ctx, repository.GetAssignmentParams{
			ID:       assignmentUUID,
			ViewerID: nil,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		testDefinition, err := repo.GetTestDefinition(ctx, assignment.TestCode)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		p3Definition, err := models.LoadP3Definition(testDefinition.SourcePath)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		startUnix, err := strconv.ParseInt(c.FormValue("start"), 10, 64)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		startTime := time.Unix(startUnix, 0)

		duration := int(time.Since(startTime).Milliseconds())

		var score int
		var responses map[string]string = make(map[string]string)

		for i := range 44 {

			asw, err := strconv.Atoi(c.FormValue(fmt.Sprintf("A%d", i)))
			if err != nil {
				return herr.Handle(c.Response(), http.StatusBadRequest, err)
			}

			responses[fmt.Sprintf("Q%d", i)] = p3Definition.Content.Questions[i][asw-1]

			score += asw

		}

		score = score - p3Definition.Adjust

		rawPercentage := float64(score) / float64(p3Definition.MaxScore) * 100.0

		var gravity string
		if rawPercentage <= 30 {
			gravity = "normal"
		} else if rawPercentage >= 31 && rawPercentage <= 50 {
			gravity = "moderate"
		} else {
			gravity = "severe"
		}

		percentage := fmt.Sprintf("%.2f", rawPercentage) + "%"

		responsesRawJson, err := json.Marshal(responses)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		assignmentResult, err := repo.UpdateAssignmentResponses(ctx, repository.UpdateAssignmentResponsesParams{
			AssignmentID: assignment.ID,
			NewResponses: responsesRawJson,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		rawScores := make(map[string]any)
		rawScores["TOTAL"] = map[string]any{
			"gravity":    gravity,
			"percentage": percentage,
			"score":      score,
		}

		rawScoresJSON, err := json.Marshal(rawScores)
		if err != nil {
			return herr.Handle(
				c.Response(),
				http.StatusInternalServerError,
				fmt.Errorf("failed to marshal raw scores: %w", err),
			)
		}

		assignmentResult, err = repo.FinalizeAssignmentResultScoring(ctx, repository.FinalizeAssignmentResultScoringParams{
			AssignmentID:    assignmentUUID,
			RawScores:       rawScoresJSON,
			DurationSeconds: new(int32(duration / 1000)),
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		indication := createBasicIndication(p3Definition.Name, duration, percentage, gravity)

		log.Debugf("Indication: %s", indication)

		_, err = repo.CreateSumBandResult(ctx, repository.CreateSumBandResultParams{
			AssignmentResultID: assignmentResult.ID,
			HeadlineScore:      percentage,
			NumericScore: pgtype.Numeric{
				Int:   big.NewInt(int64(score)),
				Valid: true,
			},
			ScaleLabel:            fmt.Sprintf("%d out of %d", score, p3Definition.MaxScore),
			SeverityLabel:         gravity,
			InterpretationSummary: indication,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to create sum band result: %w", err))
		}

		_, err = repo.MarkAssignmentReadyForReview(ctx, assignment.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark assignment ready for review: %w", err))
		}

		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/examinating/%s?token=%s", assignment.ExaminationID.String(), token))
	}
}

func DASS42Calc() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("calculating DASS42 results", "DASS42Calc", c.Request().Header.Get("X-Request-ID"))
		assignmentID := c.Param("id")
		assignmentUUID, err := uuid.Parse(assignmentID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		token := c.QueryParam("token")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)

		repo := repository.New(tx)

		assignment, err := repo.GetAssignment(ctx, repository.GetAssignmentParams{
			ID:       assignmentUUID,
			ViewerID: nil,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		testDefinition, err := repo.GetTestDefinition(ctx, assignment.TestCode)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		dass42Definition, err := models.LoadDass42Definition(testDefinition.SourcePath)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		startUnix, err := strconv.ParseInt(c.FormValue("start"), 10, 64)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid start time: %w", err))
		}
		startTime := time.Unix(startUnix, 0)

		duration := int(time.Since(startTime).Milliseconds())

		var scores map[string]*models.SubResult = make(map[string]*models.SubResult, len(dass42Definition.Subscales))
		for _, subscale := range dass42Definition.Subscales {
			scores[subscale.Code] = &models.SubResult{
				Score:      0,
				Gravity:    "",
				Percentage: "",
				Indication: "",
			}
		}

		var responses map[string]string = make(map[string]string)

		for i := range dass42Definition.Items {

			asw, err := strconv.Atoi(c.FormValue(fmt.Sprintf("q%d", i)))
			if err != nil {
				return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid answer: %w", err))
			}

			switch asw {
			case 0:
				responses[dass42Definition.Content.Questions[i]] = "Did not apply to me at all"
			case 1:
				responses[dass42Definition.Content.Questions[i]] = "Mildly but it did't bother me much"
			case 2:
				responses[dass42Definition.Content.Questions[i]] = "Applied to me a considerable degree, or a good part of time"
			case 3:
				responses[dass42Definition.Content.Questions[i]] = "Applied to me very much, or most of the time"
			default:
				return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid answer: %w", err))
			}

			code := dass42Definition.GetSubscaleCodeFromItemKey(i + 1)
			if code != "" {
				scores[code].Score += asw
			}

		}

		responsesRawJson, err := json.Marshal(responses)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		assignmentResult, err := repo.UpdateAssignmentResponses(ctx, repository.UpdateAssignmentResponsesParams{
			AssignmentID: assignment.ID,
			NewResponses: responsesRawJson,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		rawScores := make(map[string]any, len(dass42Definition.Subscales))
		for _, subscale := range dass42Definition.Subscales {
			result := scores[subscale.Code]

			rawPercentage := float64(result.Score) / float64(subscale.MaxScore) * 100
			result.Percentage = fmt.Sprintf("%.2f%%", rawPercentage)

			for _, band := range subscale.Bands {
				if band.UpperBound == nil || result.Score <= *band.UpperBound {
					result.Gravity = band.Label
					break
				}
			}
			scoreName := fmt.Sprintf("%s - %s", dass42Definition.Name, subscale.Name)

			rawScores[scoreName] = map[string]any{
				"gravity":    result.Gravity,
				"percentage": result.Percentage,
				"score":      result.Score,
			}
			indication := createBasicIndication(fmt.Sprintf("%s - %s", dass42Definition.Name, subscale.Name), duration, result.Percentage, result.Gravity)

			result.Indication = indication

			_, err = repo.CreateSumBandResult(ctx, repository.CreateSumBandResultParams{
				AssignmentResultID: assignmentResult.ID,
				SubscaleCode:       subscale.Code,
				HeadlineScore:      result.Percentage,
				NumericScore: pgtype.Numeric{
					Int:   big.NewInt(int64(result.Score)),
					Valid: true,
				},
				ScaleLabel:            fmt.Sprintf("%d out of %d", result.Score, dass42Definition.Items),
				SeverityLabel:         result.Gravity,
				InterpretationSummary: result.Indication,
			})
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to create sum band result: %w", err))
			}

			scores[subscale.Code] = result
		}

		rawScoresJSON, err := json.Marshal(rawScores)
		if err != nil {
			return herr.Handle(
				c.Response(),
				http.StatusInternalServerError,
				fmt.Errorf("failed to marshal raw scores: %w", err),
			)
		}

		assignmentResult, err = repo.FinalizeAssignmentResultScoring(ctx, repository.FinalizeAssignmentResultScoringParams{
			AssignmentID:    assignmentUUID,
			RawScores:       rawScoresJSON,
			DurationSeconds: new(int32(duration / 1000)),
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		_, err = repo.MarkAssignmentReadyForReview(ctx, assignment.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark assignment ready for review: %w", err))
		}

		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/examinating/%s?token=%s", assignment.ExaminationID.String(), token))

	}
}

func BDI2Calc() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("calculating BDI-II results", "BDI2Calc", c.Request().Header.Get("X-Request-ID"))
		assignmentID := c.Param("id")
		assignmentUUID, err := uuid.Parse(assignmentID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		token := c.QueryParam("token")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)

		repo := repository.New(tx)

		assignment, err := repo.GetAssignment(ctx, repository.GetAssignmentParams{
			ID:       assignmentUUID,
			ViewerID: nil,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		testDefinition, err := repo.GetTestDefinition(ctx, assignment.TestCode)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		bdi2Definition, err := models.LoadBDI2Definition(testDefinition.SourcePath)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		startUnix, err := strconv.ParseInt(c.FormValue("start"), 10, 64)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid start time: %w", err))
		}
		startTime := time.Unix(startUnix, 0)

		duration := int(time.Since(startTime).Milliseconds())

		var score int
		responses := make(map[string]string)

		for i := range bdi2Definition.Items {
			asw, err := strconv.Atoi(c.FormValue(fmt.Sprintf("q%d", i)))
			if err != nil {
				return herr.Handle(c.Response(), http.StatusBadRequest, err)
			}

			topic := bdi2Definition.Content.Topics[i]

			if asw < 0 || asw >= len(topic.Answers) {
				return herr.Handle(
					c.Response(),
					http.StatusBadRequest,
					fmt.Errorf("invalid answer index %d for question %d", asw, i),
				)
			}

			responses[topic.Name] = topic.Answers[asw]

			score += asw
		}

		rawPercentage := float64(score) / float64(bdi2Definition.MaxScore) * 100.0

		var gravity string
		for _, band := range bdi2Definition.Bands {
			if band.UpperBound == nil || score <= *band.UpperBound {
				gravity = band.Label
				break
			}
		}

		percentage := fmt.Sprintf("%.2f", rawPercentage) + "%"

		responsesRawJson, err := json.Marshal(responses)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		assignmentResult, err := repo.UpdateAssignmentResponses(ctx, repository.UpdateAssignmentResponsesParams{
			AssignmentID: assignment.ID,
			NewResponses: responsesRawJson,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		rawScores := make(map[string]any)
		rawScores["TOTAL"] = map[string]any{
			"gravity":    gravity,
			"percentage": percentage,
			"score":      score,
		}

		rawScoresJSON, err := json.Marshal(rawScores)
		if err != nil {
			return herr.Handle(
				c.Response(),
				http.StatusInternalServerError,
				fmt.Errorf("failed to marshal raw scores: %w", err),
			)
		}

		assignmentResult, err = repo.FinalizeAssignmentResultScoring(ctx, repository.FinalizeAssignmentResultScoringParams{
			AssignmentID:    assignmentUUID,
			RawScores:       rawScoresJSON,
			DurationSeconds: new(int32(duration / 1000)),
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		indication := createBasicIndication(bdi2Definition.Name, duration, percentage, gravity)

		log.Debugf("Indication: %s", indication)

		_, err = repo.CreateSumBandResult(ctx, repository.CreateSumBandResultParams{
			AssignmentResultID: assignmentResult.ID,
			HeadlineScore:      percentage,
			NumericScore: pgtype.Numeric{
				Int:   big.NewInt(int64(score)),
				Valid: true,
			},
			ScaleLabel:            fmt.Sprintf("%d out of %d", score, bdi2Definition.MaxScore),
			SeverityLabel:         gravity,
			InterpretationSummary: indication,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to create sum band result: %w", err))
		}

		_, err = repo.MarkAssignmentReadyForReview(ctx, assignment.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark assignment ready for review: %w", err))
		}

		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/examinating/%s?token=%s", assignment.ExaminationID.String(), token))

	}
}

func PTSDCalc() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("calculating PTSD results", "PTSDCalc", c.Request().Header.Get("X-Request-ID"))
		assignmentID := c.Param("id")
		assignmentUUID, err := uuid.Parse(assignmentID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		token := c.QueryParam("token")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)

		repo := repository.New(tx)

		assignment, err := repo.GetAssignment(ctx, repository.GetAssignmentParams{
			ID:       assignmentUUID,
			ViewerID: nil,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		testDefinition, err := repo.GetTestDefinition(ctx, assignment.TestCode)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		ptsdDefinition, err := models.LoadPTSDDefinition(testDefinition.SourcePath)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		startUnix, err := strconv.ParseInt(c.FormValue("start"), 10, 64)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid start time: %w", err))
		}
		startTime := time.Unix(startUnix, 0)

		duration := int(time.Since(startTime).Milliseconds())

		var scores map[string]*models.SubResult = make(map[string]*models.SubResult, len(ptsdDefinition.Subscales))
		for _, subscale := range ptsdDefinition.Subscales {
			scores[subscale.Code] = &models.SubResult{
				Score:      0 - subscale.Adjust,
				Gravity:    "",
				Percentage: "",
				Indication: "",
			}
		}

		var responses map[string]string = make(map[string]string)

		for i := range ptsdDefinition.Items {

			asw, err := strconv.Atoi(c.FormValue(fmt.Sprintf("q%d", i)))
			if err != nil {
				return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid answer: %w", err))
			}

			switch asw {
			case 1:
				responses[ptsdDefinition.Content.Questions[i]] = "Not at all"
			case 2:
				responses[ptsdDefinition.Content.Questions[i]] = "A little bit"
			case 3:
				responses[ptsdDefinition.Content.Questions[i]] = "Moderately"
			case 4:
				responses[ptsdDefinition.Content.Questions[i]] = "Quite a bit"
			case 5:
				responses[ptsdDefinition.Content.Questions[i]] = "Extremely"
			default:
				return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid answer: %w", err))
			}

			codes := ptsdDefinition.GetSubscalesCodeFromItemKey(i + 1)
			for _, code := range codes {
				scores[code].Score += asw
			}

		}

		responsesRawJson, err := json.Marshal(responses)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		assignmentResult, err := repo.UpdateAssignmentResponses(ctx, repository.UpdateAssignmentResponsesParams{
			AssignmentID: assignment.ID,
			NewResponses: responsesRawJson,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		rawScores := make(map[string]any, len(ptsdDefinition.Subscales))
		for _, subscale := range ptsdDefinition.Subscales {
			result := scores[subscale.Code]

			slog.Debug("subscale result", "code", subscale.Code, "score", result.Score)

			rawPercentage := float64(result.Score) / float64(subscale.MaxScore) * 100
			result.Percentage = fmt.Sprintf("%.2f%%", rawPercentage)

			slog.Debug("subscale result", "code", subscale.Code, "percentage", result.Percentage)

			for _, band := range subscale.Bands {
				if band.UpperBound == nil || result.Score <= *band.UpperBound {
					result.Gravity = band.Label
					break
				}
			}
			scoreName := fmt.Sprintf("%s - %s", ptsdDefinition.Name, subscale.Name)

			slog.Debug("score name", "name", scoreName)

			rawScores[scoreName] = map[string]any{
				"gravity":    result.Gravity,
				"percentage": result.Percentage,
				"score":      result.Score,
			}
			indication := createBasicIndication(fmt.Sprintf("%s - %s", ptsdDefinition.Name, subscale.Name), duration, result.Percentage, result.Gravity)

			slog.Debug("indication", "indication", indication)

			result.Indication = indication

			_, err = repo.CreateSumBandResult(ctx, repository.CreateSumBandResultParams{
				AssignmentResultID: assignmentResult.ID,
				SubscaleCode:       subscale.Code,
				HeadlineScore:      result.Percentage,
				NumericScore: pgtype.Numeric{
					Int:   big.NewInt(int64(result.Score)),
					Valid: true,
				},
				ScaleLabel:            fmt.Sprintf("%d out of %d", result.Score, subscale.MaxScore),
				SeverityLabel:         result.Gravity,
				InterpretationSummary: result.Indication,
			})
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to create sum band result: %w", err))
			}

			scores[subscale.Code] = result
		}

		rawScoresJSON, err := json.Marshal(rawScores)
		if err != nil {
			return herr.Handle(
				c.Response(),
				http.StatusInternalServerError,
				fmt.Errorf("failed to marshal raw scores: %w", err),
			)
		}

		assignmentResult, err = repo.FinalizeAssignmentResultScoring(ctx, repository.FinalizeAssignmentResultScoringParams{
			AssignmentID:    assignmentUUID,
			RawScores:       rawScoresJSON,
			DurationSeconds: new(int32(duration / 1000)),
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		_, err = repo.MarkAssignmentReadyForReview(ctx, assignment.ID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark assignment ready for review: %w", err))
		}

		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/examinating/%s?token=%s", assignment.ExaminationID.String(), token))

	}
}

func MMPICalc() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("calculating MMPI2 results", "MMPICalc", c.Request().Header.Get("X-Request-ID"))

		assignmentID := c.Param("id")
		assignmentUUID, err := uuid.Parse(assignmentID)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		token := c.QueryParam("token")

		ctx := c.Request().Context()
		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)

		repo := repository.New(tx)

		assignment, err := repo.GetAssignment(ctx, repository.GetAssignmentParams{
			ID:       assignmentUUID,
			ViewerID: nil,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		testDefinition, err := repo.GetTestDefinition(ctx, assignment.TestCode)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		mmpi2Definition, err := models.LoadMMPIDefinition(testDefinition.SourcePath)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		answersPerPage := 25

		startUnix, err := strconv.ParseInt(c.FormValue("start"), 10, 64)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, fmt.Errorf("invalid start time: %w", err))
		}
		startTime := time.Unix(startUnix, 0)

		duration := int(time.Since(startTime).Milliseconds())

		slog.Debug("Page duration", "page", assignment.Page, "duration", duration)

		var answers []string

		var limit int
		if assignment.Page != 23 {
			limit = answersPerPage
		} else {
			limit = mmpi2Definition.Items - (answersPerPage * 22)
		}

		if boot.Environment.GoEnv != enums.Environments.DEVELOPMENT {
			for i := 0; i < limit; i++ {
				answer, err := strconv.Atoi(c.FormValue(fmt.Sprintf("%dA%d", assignment.Page, i)))
				if err != nil {
					log.Errorf("Could not get answer: %v", err)
					return echo.NewHTTPError(http.StatusBadRequest, "Insifficient data")
				}

				if answer > 1 || answer < 0 {
					log.Errorf("Could not get answer: %v", err)
					return echo.NewHTTPError(http.StatusBadRequest, "Invalid data")
				}

				if answer == 0 {

					answers = append(answers, "F")
				} else {

					answers = append(answers, "T")
				}
			}
		} else {
			// TESTING MMPI CODE ONLY

			startIndex := (int(assignment.Page) - 1) * limit
			endIndex := startIndex + limit

			test := strings.Split(MMPI_TEST_ANSWERS, "")[startIndex:endIndex]

			answers = append(answers, test...)

			// END OF TEST CODE
		}

		responsesRawJson, err := json.Marshal(map[string][]string{"answers": answers})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		as, err := repo.UpdateAssignmentResponsesMMPI(ctx, repository.UpdateAssignmentResponsesMMPIParams{
			AssignmentID: assignment.ID,
			NewResponses: responsesRawJson,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		responsesMap := make(map[string][]string)
		if err := json.Unmarshal(as.Responses, &responsesMap); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		slog.Debug("assignment responses updated", "assignment_id", as.ID, "responses count", len(responsesMap["answers"]), "responses", responsesMap["answers"])

		durationSeconds := int32(duration / 1000)
		slog.Debug("assignment duration updated", "assignment_id", as.ID, "duration", durationSeconds)

		as, err = repo.IncrementAssignmentDuration(ctx, repository.IncrementAssignmentDurationParams{
			AssignmentID:    assignment.ID,
			DurationSeconds: &durationSeconds,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		slog.Debug("assignment duration updated", "assignment_id", as.ID, "duration", as.DurationSeconds)

		err = repo.UpdateAssignmentPage(ctx, repository.UpdateAssignmentPageParams{
			AssignmentID: assignment.ID,
			Page:         int32(assignment.Page + 1),
			ViewerID:     nil,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		if assignment.Page == 23 {
			_, err = repo.MarkAssignmentReadyForReview(ctx, assignment.ID)
			if err != nil {
				return herr.Handle(c.Response(), http.StatusInternalServerError, fmt.Errorf("failed to mark assignment ready for review: %w", err))
			}

			return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/examinating/%s?token=%s", assignment.ExaminationID.String(), token))
		}

		questionsPerPage := 25
		startIndex := int(assignment.Page-1) * questionsPerPage
		endIndex := startIndex + questionsPerPage

		if startIndex < 0 || startIndex >= len(mmpi2Definition.Content.Questions) {
			return echo.NewHTTPError(404, "Page not found", http.StatusNotFound)
		}

		if endIndex > len(mmpi2Definition.Content.Questions) {
			endIndex = len(mmpi2Definition.Content.Questions)
		}

		paginatedQuestions := mmpi2Definition.Content.Questions[startIndex:endIndex]

		data := config.GetDefaultSite(c.Request())

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		html := helpers.MustRenderHTML(assignments.MMPIFormPartial(paginatedQuestions, int(assignment.Page+1), data.CSRF, data.Nonce))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}
