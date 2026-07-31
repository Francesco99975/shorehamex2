package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/labstack/echo/v4"

	"github.com/Francesco99975/shorehamex2/internal/database"

	"github.com/Francesco99975/shorehamex2/internal/helpers"
)

func main() {
	err := boot.LoadEnvVariables()
	if err != nil {
		panic(err)
	}

	slog.SetDefault(boot.NewLogger())

	if err := config.LoadManifest("./static"); err != nil {
		log.Fatalf("Failed to load Vite manifest: %v", err)
	}

	// Create a root ctx and a CancelFunc which can be used to cancel retentionMap goroutine
	rootCtx := context.Background()
	ctx, cancel := context.WithCancel(rootCtx)
	defer cancel()

	port := boot.Environment.Port

	database.Setup(boot.Environment.DSN)
	defer database.Close()
	slog.Info("Connected to database")

	e := createRouter()

	// err = tools.AddJob("cleanup", "0 0 * * *", func() {
	// 	repo := repository.New(database.Pool())
	// 	err = repo.CleanupExpiredEmailVerifications(ctx)
	// 	if err != nil {
	// 		slog.Warn("Failed to cleanup expired email verifications", slog.Any("error", err))
	// 	}
	// 	err = repo.CleanupExpiredPasswordResets(ctx)
	// 	if err != nil {
	// 		slog.Warn("Failed to cleanup expired password resets", slog.Any("error", err))
	// 	}

	// 	err = repo.DeleteExpiredPendingAuthChallenges(ctx)
	// 	if err != nil {
	// 		slog.Warn("Failed to delete expired pending auth challenges", slog.Any("error", err))
	// 	}

	// 	slog.Info("Cleanup Ran!")
	// })
	// if err != nil {
	// 	boot.FatalLog("Failed to add cleanup job", err)
	// }

	go func() {
		slog.Info("Starting Server",
			slog.String("framework", "echo"),
			slog.String("version", echo.Version),
			slog.String("port", port),
		)
		slog.Info("Local Access", slog.String("url", fmt.Sprintf("http://localhost:%s", port)))
		slog.Info("Internet Access", slog.String("url", boot.Environment.URL))
		slog.Info("Press Ctrl+C to stop the server and exit.")
		err := e.Start(":" + port)
		boot.FatalLog("Server exited, could not start", err)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	helpers.Notify("shorehamex2", "Server is shutting down")
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		helpers.Notify("shorehamex2", fmt.Sprintf("Server forced to shutdown: %v", err))
		boot.FatalLog("Server forced to shutdown", err)
	}
}
