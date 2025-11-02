package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
)

type healthCheckResponse struct {
	Status string `json:"status"`
}

func healthCheckHandler(c echo.Context) error {
	response := healthCheckResponse{
		Status: "up",
	}
	return c.JSON(http.StatusOK, response)
}

func prometheusHandler(c echo.Context) error {
	promhttp.Handler().ServeHTTP(c.Response(), c.Request())
	return nil
}

func killHandler(e *echo.Echo) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.String(http.StatusOK, "Request accepted. Service will stop shortly.")
		go func() {
			time.Sleep(500 * time.Millisecond)
			slog.Info("Service shutdown has been requested via API. Shutting down the Service now.")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			e.Shutdown(ctx)
		}()
		return nil
	}
}

func StartWebServer() {
	go func() {
		e := echo.New()
		e.HideBanner = true
		e.HidePort = true
		// e.Use(middleware.Logger())
		e.Use(middleware.Recover())
		e.GET("/health", healthCheckHandler)
		e.GET("/metrics", prometheusHandler)
		e.POST("/kill", killHandler(e))
		port := viper.Get("api.port")
		e.Logger.Fatal(e.Start(fmt.Sprintf(":%d", port)))
		slog.Info("Started Echo Webserver", "port", port)
	}()
}
