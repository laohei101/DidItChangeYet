package watcher

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/laohei101/diditchangeyet/alert"
	"github.com/laohei101/diditchangeyet/config"
)

// Watcher orchestrates a single watch: fetch → extract → compare → alert → persist.
type Watcher struct {
	cfg     *config.Watch
	state   *StateStore
	alerts  []alert.Alerter
	log     *slog.Logger
	timeout string
}

// New creates a Watcher from a watch config entry.
func New(cfg *config.Watch, globalTimeout string, state *StateStore, log *slog.Logger) (*Watcher, error) {
	timeout := cfg.Timeout
	if timeout == "" {
		timeout = globalTimeout
	}

	var alerters []alert.Alerter
	for i, a := range cfg.Alerts {
		al, err := alert.Build(a)
		if err != nil {
			return nil, fmt.Errorf("alert[%d] %q: %w", i, a.Type, err)
		}
		alerters = append(alerters, al)
	}

	return &Watcher{
		cfg:     cfg,
		state:   state,
		alerts:  alerters,
		log:     log.With("watch", cfg.ID),
		timeout: timeout,
	}, nil
}

// Run executes one check cycle: fetch → compare → alert → save.
func (w *Watcher) Run() {
	httpClient := NewHTTPClient(w.timeout)
	result := Check(w.cfg, httpClient)

	ws := w.state.Get(w.cfg.ID)
	if ws == nil {
		ws = &WatchState{}
	}

	ws.LastCheck = time.Now()
	ws.CheckCount++

	if result.Err != nil {
		errMsg := result.Err.Error()
		ws.LastError = errMsg
		w.log.Error("check failed", "error", errMsg)

		if w.cfg.Condition == "absence" {
			msg := w.buildMessage("ABSENT", ws.LastValue, "resource is unreachable")
			w.sendAlerts("ABSENT", ws.LastValue, msg)
			ws.Triggered = true
		}

		_ = w.state.Set(w.cfg.ID, ws)
		return
	}

	ws.LastError = ""
	current := result.Value
	prev := ws.LastValue
	hasPrev := prev != "" || ws.CheckCount > 1

	triggered, reason := w.evaluate(current, prev, hasPrev)

	w.log.Info("check complete",
		"value", current,
		"previous", prev,
		"triggered", triggered,
		"status_code", result.StatusCode,
	)

	if triggered {
		msg := w.buildMessage(current, prev, reason)
		w.sendAlerts(current, prev, msg)
	}

	ws.LastValue = current
	ws.Triggered = triggered
	if err := w.state.Set(w.cfg.ID, ws); err != nil {
		w.log.Error("saving state", "error", err)
	}
}

func (w *Watcher) evaluate(current, prev string, hasPrev bool) (bool, string) {
	switch w.cfg.Condition {
	case "changed":
		if !hasPrev {
			return false, ""
		}
		if current != prev {
			return true, fmt.Sprintf("value changed: %q → %q", prev, current)
		}
		return false, ""

	case "above":
		cur, err1 := strconv.ParseFloat(strings.TrimSpace(current), 64)
		thr, err2 := strconv.ParseFloat(strings.TrimSpace(w.cfg.Threshold), 64)
		if err1 != nil || err2 != nil {
			w.log.Warn("above: non-numeric value or threshold", "value", current, "threshold", w.cfg.Threshold)
			return false, ""
		}
		if cur > thr {
			return true, fmt.Sprintf("value %s is above threshold %s", current, w.cfg.Threshold)
		}
		return false, ""

	case "below":
		cur, err1 := strconv.ParseFloat(strings.TrimSpace(current), 64)
		thr, err2 := strconv.ParseFloat(strings.TrimSpace(w.cfg.Threshold), 64)
		if err1 != nil || err2 != nil {
			w.log.Warn("below: non-numeric value or threshold", "value", current, "threshold", w.cfg.Threshold)
			return false, ""
		}
		if cur < thr {
			return true, fmt.Sprintf("value %s is below threshold %s", current, w.cfg.Threshold)
		}
		return false, ""

	case "equals":
		if current == w.cfg.Threshold {
			return true, fmt.Sprintf("value equals %q", w.cfg.Threshold)
		}
		return false, ""

	case "contains":
		if strings.Contains(current, w.cfg.Threshold) {
			return true, fmt.Sprintf("value contains %q", w.cfg.Threshold)
		}
		return false, ""

	case "regex":
		re, err := regexp.Compile(w.cfg.Pattern)
		if err != nil {
			w.log.Error("invalid regex pattern", "pattern", w.cfg.Pattern, "error", err)
			return false, ""
		}
		if re.MatchString(current) {
			return true, fmt.Sprintf("value matches pattern %q", w.cfg.Pattern)
		}
		return false, ""

	case "absence":
		if current == "ABSENT" {
			return true, "value or resource is absent"
		}
		return false, ""
	}

	return false, ""
}

func (w *Watcher) buildMessage(current, prev, reason string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[http-watcher] Alert: %s\n", w.cfg.ID))
	sb.WriteString(fmt.Sprintf("URL: %s\n", w.cfg.URL))
	sb.WriteString(fmt.Sprintf("Reason: %s\n", reason))
	if prev != "" && current != prev {
		sb.WriteString(fmt.Sprintf("Previous: %s\n", prev))
		sb.WriteString(fmt.Sprintf("Current:  %s\n", current))
	} else {
		sb.WriteString(fmt.Sprintf("Value: %s\n", current))
	}
	sb.WriteString(fmt.Sprintf("Time: %s", time.Now().UTC().Format(time.RFC3339)))
	return sb.String()
}

func (w *Watcher) sendAlerts(current, prev, msg string) {
	for _, al := range w.alerts {
		if err := al.Send(msg, w.cfg.ID, current, prev); err != nil {
			w.log.Error("alert failed", "type", al.Name(), "error", err)
		} else {
			w.log.Info("alert sent", "type", al.Name())
		}
	}
}
