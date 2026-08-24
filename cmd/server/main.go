package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"elevate/internal/config"
	"elevate/internal/database"
	"elevate/internal/router"
)

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiWhite   = "\033[97m"
)

func colorize(color, s string) string {
	return color + s + ansiReset
}

func logInfo(format string, args ...any) {
	fmt.Printf(
		"%s %s\n",
		colorize(ansiCyan+ansiBold, "[INFO]"),
		fmt.Sprintf(format, args...),
	)
}

func logSuccess(format string, args ...any) {
	fmt.Printf(
		"%s %s\n",
		colorize(ansiGreen+ansiBold, "[ OK ]"),
		fmt.Sprintf(format, args...),
	)
}

func logWarn(format string, args ...any) {
	fmt.Printf(
		"%s %s\n",
		colorize(ansiYellow+ansiBold, "[WARN]"),
		fmt.Sprintf(format, args...),
	)
}

func logError(format string, args ...any) {
	fmt.Fprintf(
		os.Stderr,
		"%s %s\n",
		colorize(ansiRed+ansiBold, "[FAIL]"),
		fmt.Sprintf(format, args...),
	)
}

func printBanner(cfg *config.Config) {
	line := strings.Repeat("─", 56)

	envColor := ansiGreen

	switch cfg.Env {
	case "production":
		envColor = ansiRed
	case "staging":
		envColor = ansiYellow
	}

	fmt.Println()
	fmt.Println(
		colorize(
			ansiMagenta+ansiBold,
			"┌"+line+"┐",
		),
	)

	fmt.Printf(
		"%s  %s %s\n",
		colorize(
			ansiMagenta+ansiBold,
			"│",
		),
		colorize(
			ansiWhite+ansiBold,
			"🚀 elevate API",
		),
		colorize(
			ansiDim,
			"— speak. improve. repeat.",
		),
	)

	fmt.Println(
		colorize(
			ansiMagenta+ansiBold,
			"├"+line+"┤",
		),
	)

	fmt.Printf(
		"%s  %-12s %s\n",
		colorize(ansiMagenta+ansiBold, "│"),
		colorize(ansiCyan, "Environment"),
		colorize(envColor+ansiBold, cfg.Env),
	)

	fmt.Printf(
		"%s  %-12s %s\n",
		colorize(ansiMagenta+ansiBold, "│"),
		colorize(ansiCyan, "Port"),
		colorize(ansiWhite+ansiBold, cfg.App.Port),
	)

	fmt.Printf(
		"%s  %-12s %s\n",
		colorize(ansiMagenta+ansiBold, "│"),
		colorize(ansiCyan, "Local URL"),
		colorize(
			ansiGreen+ansiBold,
			"http://localhost:"+cfg.App.Port,
		),
	)

	fmt.Printf(
		"%s  %-12s %s\n",
		colorize(ansiMagenta+ansiBold, "│"),
		colorize(ansiCyan, "API Base"),
		colorize(ansiWhite, cfg.App.APIBaseURL),
	)

	fmt.Printf(
		"%s  %-12s %s\n",
		colorize(ansiMagenta+ansiBold, "│"),
		colorize(ansiCyan, "Frontend"),
		colorize(ansiWhite, cfg.App.FrontendURL),
	)

	fmt.Printf(
		"%s  %-12s %s\n",
		colorize(ansiMagenta+ansiBold, "│"),
		colorize(ansiCyan, "Started"),
		colorize(
			ansiWhite,
			time.Now().Format(time.RFC1123),
		),
	)

	fmt.Println(
		colorize(
			ansiMagenta+ansiBold,
			"├"+line+"┤",
		),
	)

	fmt.Printf(
		"%s  %s\n",
		colorize(
			ansiMagenta+ansiBold,
			"│",
		),
		colorize(
			ansiCyan,
			"Integrations",
		),
	)

	printIntegrationRows(cfg)

	fmt.Println(
		colorize(
			ansiMagenta+ansiBold,
			"└"+line+"┘",
		),
	)

	fmt.Println()
}

func printIntegrationRows(cfg *config.Config) {
	integrations := cfg.Integrations()

	names := make([]string, 0, len(integrations))

	for name := range integrations {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		status := colorize(
			ansiRed+ansiBold,
			"✗ not configured",
		)

		if integrations[name] {
			status = colorize(
				ansiGreen+ansiBold,
				"✓ configured",
			)
		}

		fmt.Printf(
			"%s    %-10s %s\n",
			colorize(
				ansiMagenta+ansiBold,
				"│",
			),
			colorize(
				ansiWhite,
				name,
			),
			status,
		)
	}
}

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	logInfo("Loading configuration...")

	cfg, err := config.Load()
	if err != nil {
		logError(
			"configuration error: %v",
			err,
		)
		os.Exit(1)
	}

	logSuccess(
		"Configuration loaded (env=%s)",
		cfg.Env,
	)

	for name, ok := range cfg.Integrations() {
		if !ok {
			logWarn(
				"integration not configured: %s",
				name,
			)
		}
	}

	logInfo("Connecting to database...")

	db, err := connectDBWithRetry(
		ctx,
		cfg,
		5,
		2*time.Second,
	)
	if err != nil {
		logError(
			"could not connect to database: %v",
			err,
		)
		os.Exit(1)
	}

	defer func() {
		logInfo("Closing database pool...")
		db.Close()
	}()

	logSuccess(
		"Database connected (pool: min=%d max=%d)",
		cfg.DB.MinConns,
		cfg.DB.MaxConns,
	)

	logInfo("Connecting to Redis...")

	redisClient, err := connectRedisWithRetry(
		ctx,
		cfg,
		5,
		2*time.Second,
	)
	if err != nil {
		logError(
			"could not connect to redis: %v",
			err,
		)
		os.Exit(1)
	}

	defer func() {
		logInfo("Closing Redis connection...")
		_ = redisClient.Close()
	}()

	logSuccess("Redis connected")

	r := router.New(
		ctx,
		cfg,
		db,
		redisClient,
	)

	srv := &http.Server{
		Addr:         ":" + cfg.App.Port,
		Handler:      r,
		ReadTimeout:  cfg.App.ReadTimeout,
		WriteTimeout: cfg.App.WriteTimeout,
		IdleTimeout:  cfg.App.IdleTimeout,
	}

	serverErrCh := make(chan error, 1)

	go func() {
		logInfo(
			"Starting HTTP server on port %s...",
			cfg.App.Port,
		)

		if err := srv.ListenAndServe(); err != nil &&
			!errors.Is(
				err,
				http.ErrServerClosed,
			) {
			serverErrCh <- err
			return
		}

		serverErrCh <- nil
	}()

	select {
	case err := <-serverErrCh:
		if err != nil {
			logError(
				"server failed to start: %v",
				err,
			)
			os.Exit(1)
		}

	case <-time.After(
		150 * time.Millisecond,
	):
		printBanner(cfg)
	}

	select {
	case <-ctx.Done():
		logWarn(
			"Shutdown signal received, draining connections...",
		)

	case err := <-serverErrCh:
		if err != nil {
			logError(
				"server crashed: %v",
				err,
			)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.App.ShutdownTimeout,
	)
	defer cancel()

	if err := srv.Shutdown(
		shutdownCtx,
	); err != nil {
		logError(
			"graceful shutdown failed: %v",
			err,
		)
		os.Exit(1)
	}

	logSuccess(
		"Server shut down cleanly. Goodbye 👋",
	)
}

func connectDBWithRetry(
	ctx context.Context,
	cfg *config.Config,
	attempts int,
	backoff time.Duration,
) (*database.DB, error) {
	var lastErr error

	for i := 1; i <= attempts; i++ {
		db, err := database.Connect(
			ctx,
			cfg.DB,
		)

		if err == nil {
			return db, nil
		}

		lastErr = err

		logWarn(
			"database connection attempt %d/%d failed: %v",
			i,
			attempts,
			err,
		)

		if i < attempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()

			case <-time.After(backoff):
			}
		}
	}

	return nil, lastErr
}

func connectRedisWithRetry(
	ctx context.Context,
	cfg *config.Config,
	attempts int,
	backoff time.Duration,
) (*redis.Client, error) {
	var lastErr error

	for i := 1; i <= attempts; i++ {
		client, err := database.ConnectRedis(
			ctx,
			cfg.Redis,
		)

		if err == nil {
			return client, nil
		}

		lastErr = err

		logWarn(
			"redis connection attempt %d/%d failed: %v",
			i,
			attempts,
			err,
		)

		if i < attempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()

			case <-time.After(backoff):
			}
		}
	}

	return nil, lastErr
}
