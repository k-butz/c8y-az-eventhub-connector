package monitoring

import (
	"log/slog"
	"math"
	"runtime"
	"time"

	"github.com/robfig/cron/v3"
	gcpu "github.com/shirou/gopsutil/cpu"
	gmem "github.com/shirou/gopsutil/mem"
	"github.com/spf13/viper"
)

func StartResourceLogger() {
	// run in background
	go func() {
		c := cron.New()
		cronExpression := viper.GetString("monitoring.console_cron")
		c.AddFunc(cronExpression, func() {
			mem, _ := gmem.VirtualMemory()
			cpuPercentTotal, _ := gcpu.Percent(3*time.Second, false)
			cpuPercents, _ := gcpu.Percent(3*time.Second, true)
			cpuPhysicalCount, _ := gcpu.Counts(false)
			cpuLogicalCount, _ := gcpu.Counts(true)
			slog.Info("Resource Monitor collected statistics",
				"ctGoRoutines", runtime.NumGoroutine(),
				"memTotal", mem.Total,
				"memAvailable", mem.Available,
				"memUsedPercent", round(mem.UsedPercent, 3),
				"cpuPhysicalCount", cpuPhysicalCount,
				"cpuLogicalCount", cpuLogicalCount,
				"cpuPercentTotal", toRoundedFloats(cpuPercentTotal),
				"cpuPercents", toRoundedFloats(cpuPercents),
			)
		})
		c.Start()
		slog.Info("Started Cron for Resource Monitoring", "cronExpression", cronExpression)
	}()
}

func toRoundedFloats(elements []float64) []float64 {
	res := []float64{}
	for _, e := range elements {
		res = append(res, round(e, 3))
	}
	return res
}

func round(val float64, n int) float64 {
	pow := math.Pow(10, float64(n))
	scaledVal := val * pow
	roundedScaled := math.Round(scaledVal)
	return roundedScaled / pow
}
