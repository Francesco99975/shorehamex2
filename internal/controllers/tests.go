package controllers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

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
	"github.com/labstack/echo/v4"
)

func Tests() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("tests", "Tests", c.Request().Header.Get("X-Request-ID"))
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

		data := config.GetDefaultSite(c.Request())
		data.CurrentUser = user

		data.Nonce = c.Get("nonce").(string)
		data.CSRF = c.Get("csrf").(string)

		userID, err := uuid.Parse(user.ID)
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}

		slog.Debug("Authenticated user ID", slog.String("userID", userID.String()))

		testDefinitions, err := repo.ListTestDefinitions(ctx, repository.ListTestDefinitionsParams{
			ActiveOnly: nil,
			Domain:     nil,
			RowOffset:  0,
			RowLimit:   50,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		slog.Debug("Test definitions", slog.Int("count", len(testDefinitions)))

		testDefinitionViews := helpers.MapSlice(testDefinitions, func(testDefinition *repository.ListTestDefinitionsRow) models.TestDefinitionView {
			data, _ := os.ReadFile(testDefinition.SourcePath)

			var td models.TestDefinition
			_ = json.Unmarshal(data, &td)

			return models.TestDefinitionView{
				TestDefinition:  td,
				AssignmentCount: int(testDefinition.AssignmentCount),
			}
		})

		slog.Debug("Test definitions", slog.Int("count", len(testDefinitionViews)))

		html := helpers.MustRenderHTML(views.TestCatalog(data, testDefinitionViews))

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func UploadTest() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("uploading test", "UploadTest", c.Request().Header.Get("X-Request-ID"))

		file, err := c.FormFile("test_file")
		if err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		src, err := file.Open()
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer src.Close()

		data, err := io.ReadAll(src)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		var testDefinition models.TestDefinition
		if err := json.Unmarshal(data, &testDefinition); err != nil {
			return herr.Handle(c.Response(), http.StatusBadRequest, err)
		}

		savePath := "data/tests"
		if err := os.MkdirAll(savePath, 0755); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		filename := filepath.Base(file.Filename)
		dst, err := os.Create(filepath.Join(savePath, filename))
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}
		defer dst.Close()

		if _, err := dst.Write(data); err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		ctx := c.Request().Context()

		tx, err := database.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return herr.HandleEchoPage(http.StatusInternalServerError, err)
		}
		defer database.HandleTransaction(ctx, tx, &err)
		repo := repository.New(tx)

		definition, err := repo.CreateTestDefinition(ctx, repository.CreateTestDefinitionParams{
			Code:                   testDefinition.Code,
			Name:                   testDefinition.Name,
			Domain:                 repository.TestDomain(testDefinition.TestDomain),
			ScoringMethod:          repository.ScoringMethod(testDefinition.ScoringMethod),
			RequiresSex:            testDefinition.RequiresSex,
			ItemCount:              int16(testDefinition.Items),
			TypicalDurationMinutes: int16(testDefinition.TypicalDurationMin),
			SourcePath:             filepath.Join(savePath, filename),
			SourceChecksum:         helpers.ComputeFileChecksum(data),
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		slog.Debug("test definition created", "code", definition.Code, "name", definition.Name, "source_checksum", definition.SourceChecksum)

		d, err := os.ReadFile(definition.SourcePath)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		var td models.TestDefinition
		err = json.Unmarshal(d, &td)
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		definitionView := models.TestDefinitionView{
			TestDefinition:  td,
			AssignmentCount: 0,
		}

		tools.SetToastTrigger(c.Response(), enums.SuccessToast, "Test definition created successfully")

		html := helpers.MustRenderHTML(components.TestItem(&definitionView))
		html = append(html, helpers.MustRenderHTML(views.TestCountPartial(int(definition.TotalCount)+1, true))...)
		html = append(html, helpers.MustRenderHTML(components.EmptyState(definition.TotalCount == 0, true, "No test definitions found"))...)

		return c.Blob(http.StatusOK, "text/html", html)

	}
}

func TestOptions() echo.HandlerFunc {
	return func(c echo.Context) error {
		herr := httperr.New("listing test options", "TestOptions", c.Request().Header.Get("X-Request-ID"))
		repo := repository.New(database.Pool())

		tests, err := repo.ListTestDefinitions(c.Request().Context(), repository.ListTestDefinitionsParams{
			ActiveOnly: nil,
			Domain:     nil,
			RowOffset:  0,
			RowLimit:   50,
		})
		if err != nil {
			return herr.Handle(c.Response(), http.StatusInternalServerError, err)
		}

		instruments := helpers.MapSlice(tests, func(test *repository.ListTestDefinitionsRow) components.Instrument {
			return components.Instrument{
				Code:     test.Code,
				Name:     test.Name,
				Duration: strconv.Itoa(int(test.TypicalDurationMinutes)) + " min.",
			}
		})

		html := helpers.MustRenderHTML(components.Instruments(instruments))

		return c.Blob(http.StatusOK, "text/html", html)
	}
}
