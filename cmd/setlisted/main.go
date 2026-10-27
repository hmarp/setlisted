// Command setlisted runs the Setlisted web server.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/hmarp/setlisted/internal/config"
	"github.com/hmarp/setlisted/internal/httpapi"
	"github.com/hmarp/setlisted/web"
)

// shutdownTimeout is how long in-flight requests get to finish after SIGTERM.
// Cloud Run allows 10 seconds before it kills the container.
const shutdownTimeout = 8 * time.Second

func main() {
	logger := newLogger()
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("setlisted stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	// A local .env is optional. godotenv.Load never overrides variables that
	// are already set in the environment.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading .env: %w", err)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	srv := httpapi.NewServer(net.JoinHostPort("", cfg.Port), logger, web.Static())

	errc := make(chan error, 1)
	go func() {
		logger.Info("listening", "port", cfg.Port, "base_url", cfg.BaseURL)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// newLogger returns a JSON logger whose field names Cloud Logging recognises
// ("severity" and "message").
func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) > 0 {
				return a
			}
			switch a.Key {
			case slog.LevelKey:
				a.Key = "severity"
				if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == slog.LevelWarn {
					a.Value = slog.StringValue("WARNING")
				}
			case slog.MessageKey:
				a.Key = "message"
			}
			return a
		},
	}))
}
