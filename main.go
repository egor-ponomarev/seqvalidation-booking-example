package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/egor-ponomarev/seqvalidation-booking-example/internal/booking"
	"github.com/egor-ponomarev/seqvalidation-booking-example/internal/httpapi"

	"github.com/gin-gonic/gin"
)

const (
	serverAddress = "127.0.0.1:8080"
	logKeyError   = "error"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", logKeyError, err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	gin.SetMode(gin.ReleaseMode)

	repo := booking.NewMemoryRepository(logger)
	createBookingValidator, err := booking.NewCreateValidator(repo)
	if err != nil {
		return err
	}
	bookingService := booking.NewService(repo)

	server := &http.Server{
		Addr:              serverAddress,
		Handler:           httpapi.NewRouter(bookingService, createBookingValidator, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	logger.Info("booking example listening", "address", serverAddress)

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	}
}
