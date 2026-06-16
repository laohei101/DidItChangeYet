package alert

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/laohei101/diditchangeyet/config"
)

// Ntfy sends alerts to an ntfy.sh server (or self-hosted instance).
type Ntfy struct {
	server   string
	topic    string
	priority string
	client   *http.Client
}

func NewNtfy(cfg config.AlertConfig) (*Ntfy, error) {
	if cfg.Topic == "" {
		return nil, fmt.Errorf("ntfy: topic is required")
	}
	server := cfg.Server
	if server == "" {
		server = "https://ntfy.sh"
	}
	priority := cfg.Priority
	if priority == "" {
		priority = "default"
	}
	return &Ntfy{
		server:   strings.TrimRight(server, "/"),
		topic:    cfg.Topic,
		priority: priority,
		client:   &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (n *Ntfy) Name() string { return "ntfy" }

func (n *Ntfy) Send(message, watchID, current, previous string) error {
	url := fmt.Sprintf("%s/%s", n.server, n.topic)

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("ntfy: creating request: %w", err)
	}
	req.Header.Set("Title", fmt.Sprintf("Watch Alert: %s", watchID))
	req.Header.Set("Priority", n.priority)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ntfy: server returned %d: %s", resp.StatusCode, body)
	}

	return nil
}
