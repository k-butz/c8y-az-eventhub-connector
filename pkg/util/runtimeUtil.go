package util

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

func Shutdown(sleepTimeSecs, exitCode int) {
	slog.Info(fmt.Sprintf("Waiting %d seconds, then shut down", sleepTimeSecs))
	time.Sleep(time.Duration(sleepTimeSecs) * time.Second)
	os.Exit(exitCode)
}
