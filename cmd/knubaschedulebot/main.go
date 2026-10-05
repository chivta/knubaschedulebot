package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/chivta/knubaschedulebot/internal/bot"
	"github.com/chivta/knubaschedulebot/internal/cache"
	"github.com/chivta/knubaschedulebot/internal/config"
	"github.com/chivta/knubaschedulebot/internal/health"
	"github.com/chivta/knubaschedulebot/internal/logging"
	"github.com/chivta/knubaschedulebot/internal/metrics"
	"github.com/chivta/knubaschedulebot/internal/mkr"
	"github.com/chivta/knubaschedulebot/internal/storage"
)

// components is how many long-running goroutines run collects errors from.
const components = 2

func main() {
	cfg, err := config.Load()
	if err != nil {
		// The logger is not configured yet, and a config failure is exactly the
		// kind of thing that must be visible even when logging is broken.
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logging.Init(cfg.LogLevel)

	err = run(cfg)
	if err != nil {
		log.Error().Err(err).Msg("knubaschedulebot exited with an error")
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// A cancellable child of the signal context, so a failure during startup can
	// bring down whatever is already running.
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, components)

	// The probe server goes up first and stays up for the whole run, so the
	// container has something listening while the rest starts.
	probes := health.New(cfg.HTTPAddr, metrics.Handler())
	wg.Go(func() { errs <- probes.Run(ctx) })

	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return shutdown(cancel, &wg, errs, err)
	}
	defer db.Close()

	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return shutdown(cancel, &wg, errs, fmt.Errorf("parse redis url: %w", err))
	}

	// The client connects lazily, and the cache falls back to the schedule
	// site when Redis is unreachable, so a Redis outage does not stop startup.
	rdb := redis.NewClient(redisOptions)
	defer rdb.Close()

	schedule := cache.NewSchedule(rdb, mkr.New(cfg.MKRBaseURL, cfg.MKRTimeout), cfg.CacheTTL)

	telegram, err := bot.New(
		bot.Settings{Token: cfg.BotToken, APIURL: cfg.TelegramAPIURL, Admins: cfg.AdminIDs},
		schedule,
		storage.NewUserRepo(db),
		storage.NewAccessRepo(db),
	)
	if err != nil {
		return shutdown(cancel, &wg, errs, fmt.Errorf("create telegram bot: %w", err))
	}

	wg.Go(func() { errs <- telegram.Run(ctx) })

	return shutdown(nil, &wg, errs, nil)
}

// shutdown waits for every started component to return and folds their errors
// together with cause. Passing a non-nil cancel stops them first, which is what
// a startup failure needs; a nil cancel means the components are already winding
// down on their own.
func shutdown(cancel context.CancelFunc, wg *sync.WaitGroup, errs chan error, cause error) error {
	if cancel != nil {
		cancel()
	}

	wg.Wait()
	close(errs)

	joined := cause
	for err := range errs {
		joined = errors.Join(joined, err)
	}

	log.Info().Msg("shutdown complete")

	return joined
}
