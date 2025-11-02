package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/k-butz/az-eventhub-connector/pkg/util"
	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/spf13/viper"
)

func ConfigureLogger() {
	c8y.SilenceLogger()

	lvl := slog.LevelInfo
	lvlString := strings.ToUpper(viper.GetString("logs.level"))
	switch lvlString {
	case "DEBUG":
		lvl = slog.LevelDebug
	case "INFO":
		lvl = slog.LevelInfo
	case "WARN":
		lvl = slog.LevelWarn
	case "ERROR":
		lvl = slog.LevelError
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     lvl,
		AddSource: true,
		// adapt timestamp format & show only source filename instead of path
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				return slog.String(slog.TimeKey, util.ToRFCTimeStamp(a.Value.Time()))
			}
			if a.Key == slog.SourceKey {
				source, ok := a.Value.Any().(*slog.Source)
				if ok {
					source.File = filepath.Base(source.File)
				}
			}
			return a
		},
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
}
