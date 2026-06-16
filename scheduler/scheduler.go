package scheduler

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/laohei101/diditchangeyet/config"
	"github.com/laohei101/diditchangeyet/watcher"
)

// Scheduler manages the cron-based execution of all watchers.
type Scheduler struct {
	cron    *cron.Cron
	cfg     *config.Config
	state   *watcher.StateStore
	log     *slog.Logger
}

func New(cfg *config.Config, state *watcher.StateStore, log *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:  cron.New(),
		cfg:   cfg,
		state: state,
		log:   log,
	}
}

// Start registers all watches with the cron scheduler and starts it.
func (s *Scheduler) Start() error {
	for i := range s.cfg.Watches {
		w := &s.cfg.Watches[i]

		interval := w.Interval
		if interval == "" {
			interval = s.cfg.GlobalInterval
		}

		spec, err := toSchedule(interval)
		if err != nil {
			return fmt.Errorf("watch %q: invalid interval %q: %w", w.ID, interval, err)
		}

		wt, err := watcher.New(w, s.cfg.GlobalTimeout, s.state, s.log)
		if err != nil {
			return fmt.Errorf("initialising watch %q: %w", w.ID, err)
		}

		if _, err := s.cron.AddFunc(spec, wt.Run); err != nil {
			return fmt.Errorf("scheduling watch %q: %w", w.ID, err)
		}

		s.log.Info("watch scheduled", "id", w.ID, "interval", interval)
	}

	s.cron.Start()
	return nil
}

// RunOnce executes every watch exactly once and returns.
func (s *Scheduler) RunOnce() {
	for i := range s.cfg.Watches {
		w := &s.cfg.Watches[i]
		wt, err := watcher.New(w, s.cfg.GlobalTimeout, s.state, s.log)
		if err != nil {
			s.log.Error("initialising watch", "id", w.ID, "error", err)
			continue
		}
		s.log.Info("running watch", "id", w.ID)
		wt.Run()
	}
}

// Stop gracefully halts the scheduler, waiting for in-flight runs to finish.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// toSchedule converts a duration string (e.g. "5m") or a cron expression
// (e.g. "*/5 * * * *" or "@hourly") into a spec accepted by robfig/cron.
func toSchedule(interval string) (string, error) {
	interval = strings.TrimSpace(interval)
	if interval == "" {
		return "", fmt.Errorf("interval is empty")
	}

	// Cron expressions contain spaces or start with @
	if strings.ContainsAny(interval, " \t") || strings.HasPrefix(interval, "@") {
		return interval, nil
	}

	// Otherwise treat as a Go duration string
	if _, err := time.ParseDuration(interval); err != nil {
		return "", fmt.Errorf("not a valid duration or cron expression: %w", err)
	}

	return "@every " + interval, nil
}
