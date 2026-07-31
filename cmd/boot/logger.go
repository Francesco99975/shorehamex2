package boot

import (
	"log/slog"
	"os"

	"github.com/Francesco99975/shorehamex2/internal/enums"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

func NewLogger() *slog.Logger {
	var zapCore zapcore.Core

	if Environment.GoEnv == enums.Environments.PRODUCTION {
		// JSON encoder, no dev extras, max speed
		cfg := zap.NewProductionEncoderConfig()
		cfg.TimeKey = "time"
		cfg.MessageKey = "message"
		cfg.LevelKey = "level"
		cfg.CallerKey = "caller"

		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(cfg),
			zapcore.AddSync(os.Stdout),
			zapcore.InfoLevel,
		)
		zapCore = core
	} else {
		// Still JSON in dev — Loki/Promtail expect it
		// But with debug level and color-friendly field names
		cfg := zap.NewDevelopmentEncoderConfig()
		cfg.TimeKey = "time"
		cfg.MessageKey = "message"
		cfg.LevelKey = "level"
		cfg.CallerKey = "caller"
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder

		core := zapcore.NewCore(
			zapcore.NewConsoleEncoder(cfg),
			zapcore.AddSync(os.Stdout),
			zapcore.DebugLevel,
		)
		zapCore = core
	}

	zapLogger := zap.New(zapCore, zap.AddCaller())
	handler := zapslog.NewHandler(zapCore, zapslog.WithCaller(true))

	_ = zapLogger // expose if you need zap directly for sugared logger etc.

	return slog.New(handler)
}

func FatalLog(msg string, err error) {
	slog.Error(msg, slog.Any("error", err))
	os.Exit(1)
}
