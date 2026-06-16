package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/laohei101/diditchangeyet/config"
	"github.com/laohei101/diditchangeyet/dashboard"
	"github.com/laohei101/diditchangeyet/scheduler"
	"github.com/laohei101/diditchangeyet/watcher"
)

const version = "0.1.0"

func main() {
	var (
		configFile = flag.String("config", "config.yaml", "path to config file")
		runOnce    = flag.Bool("once", false, "run all checks once and exit (useful for testing)")
		logJSON    = flag.Bool("log-json", false, "emit logs as JSON instead of human-readable text")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		os.Stdout.WriteString("http-watcher v" + version + "\n")
		return
	}

	// ---------- logger ----------
	lvl := &slog.LevelVar{}
	lvl.Set(slog.LevelInfo)

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: lvl}
	if *logJSON {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	log := slog.New(handler)

	// ---------- config ----------
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Error("loading config", "file", *configFile, "error", err)
		os.Exit(1)
	}

	// Honour the log level from config (CLI flag -log-json wins for format).
	switch cfg.LogLevel {
	case "debug":
		lvl.Set(slog.LevelDebug)
	case "warn":
		lvl.Set(slog.LevelWarn)
	case "error":
		lvl.Set(slog.LevelError)
	}

	if cfg.LogFormat == "json" && !*logJSON {
		// Recreate handler if config requests JSON but flag wasn't set
		handler = slog.NewJSONHandler(os.Stdout, opts)
		log = slog.New(handler)
	}

	log.Info("http-watcher starting", "version", version, "watches", len(cfg.Watches))

	// ---------- state ----------
	state, err := watcher.NewStateStore(cfg.StateFile)
	if err != nil {
		log.Error("opening state store", "path", cfg.StateFile, "error", err)
		os.Exit(1)
	}

	// ---------- run-once mode ----------
	if *runOnce {
		sched := scheduler.New(cfg, state, log)
		sched.RunOnce()
		log.Info("run-once complete")
		return
	}

	// ---------- dashboard ----------
	if cfg.Dashboard.Enabled {
		dash := dashboard.New(cfg, state, log)
		go dash.Start(cfg.Dashboard.Port)
	}

	// ---------- scheduler ----------
	sched := scheduler.New(cfg, state, log)
	if err := sched.Start(); err != nil {
		log.Error("starting scheduler", "error", err)
		os.Exit(1)
	}

	log.Info("daemon running — press Ctrl+C to stop")
	waitForShutdown()

	log.Info("shutdown signal received, stopping...")
	sched.Stop()
	log.Info("stopped cleanly")
}
