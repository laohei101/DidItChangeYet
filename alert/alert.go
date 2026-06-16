package alert

import (
	"fmt"

	"github.com/laohei101/diditchangeyet/config"
)

// Alerter is the interface every notification channel must implement.
type Alerter interface {
	// Name returns the channel type label (e.g. "telegram").
	Name() string
	// Send delivers the alert message. watchID, current, and previous are
	// provided so implementations can format richer messages if needed.
	Send(message, watchID, current, previous string) error
}

// Build constructs the concrete Alerter for the given AlertConfig.
func Build(cfg config.AlertConfig) (Alerter, error) {
	switch cfg.Type {
	case "telegram":
		return NewTelegram(cfg)
	case "email":
		return NewEmail(cfg)
	case "ntfy":
		return NewNtfy(cfg)
	default:
		return nil, fmt.Errorf("unknown alert type %q (must be telegram, email, or ntfy)", cfg.Type)
	}
}
