package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

type Config struct {
	GlobalInterval string       `yaml:"global_interval"`
	GlobalTimeout  string       `yaml:"global_timeout"`
	StateFile      string       `yaml:"state_file"`
	LogFormat      string       `yaml:"log_format"` // "text" or "json"
	LogLevel       string       `yaml:"log_level"`  // "debug", "info", "warn", "error"
	Dashboard      DashboardCfg `yaml:"dashboard"`
	Watches        []Watch      `yaml:"watches"`
}

type DashboardCfg struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

type Watch struct {
	ID          string            `yaml:"id"`
	URL         string            `yaml:"url"`
	Method      string            `yaml:"method"`
	Headers     map[string]string `yaml:"headers"`
	Body        string            `yaml:"body"`
	Type        string            `yaml:"type"`        // json, html, http_status, text
	JSONPath    string            `yaml:"json_path"`   // gjson path for type=json
	CSSSelector string            `yaml:"css_selector"` // for type=html
	Attribute   string            `yaml:"attribute"`   // HTML attribute to extract (empty = text content)
	Condition   string            `yaml:"condition"`   // changed, above, below, equals, contains, regex, absence
	Threshold   string            `yaml:"threshold"`   // numeric or string value for comparison
	Pattern     string            `yaml:"pattern"`     // regex pattern for condition=regex
	Interval    string            `yaml:"interval"`    // cron expr or duration string
	Timeout     string            `yaml:"timeout"`     // per-watch HTTP timeout
	Alerts      []AlertConfig     `yaml:"alerts"`
}

type AlertConfig struct {
	Type string `yaml:"type"` // telegram, email, ntfy

	// Telegram
	Token  string `yaml:"token"`
	ChatID string `yaml:"chat_id"`

	// Email (SMTP)
	SMTPHost string `yaml:"smtp_host"`
	SMTPPort int    `yaml:"smtp_port"`
	TLS      bool   `yaml:"tls"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
	To       string `yaml:"to"`

	// ntfy
	Server   string `yaml:"server"`
	Topic    string `yaml:"topic"`
	Priority string `yaml:"priority"`
}

var envPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// Load reads and parses the config file, expanding {{ENV_VAR}} placeholders.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}

	expanded := envPattern.ReplaceAllStringFunc(string(data), func(m string) string {
		key := strings.TrimSpace(m[2 : len(m)-2])
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return m
	})

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.GlobalInterval == "" {
		cfg.GlobalInterval = "5m"
	}
	if cfg.GlobalTimeout == "" {
		cfg.GlobalTimeout = "30s"
	}
	if cfg.StateFile == "" {
		cfg.StateFile = "state.json"
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = "text"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.Dashboard.Port == 0 {
		cfg.Dashboard.Port = 8080
	}
	for i := range cfg.Watches {
		w := &cfg.Watches[i]
		if w.Method == "" {
			w.Method = "GET"
		}
		if w.Type == "" {
			w.Type = "text"
		}
		if w.Condition == "" {
			w.Condition = "changed"
		}
	}
}

func validate(cfg *Config) error {
	seen := make(map[string]bool)
	for i, w := range cfg.Watches {
		if w.ID == "" {
			return fmt.Errorf("watch[%d]: id is required", i)
		}
		if seen[w.ID] {
			return fmt.Errorf("watch[%d]: duplicate id %q", i, w.ID)
		}
		seen[w.ID] = true

		if w.URL == "" {
			return fmt.Errorf("watch %q: url is required", w.ID)
		}

		switch w.Type {
		case "json", "html", "http_status", "text":
		default:
			return fmt.Errorf("watch %q: unknown type %q (must be json, html, http_status, or text)", w.ID, w.Type)
		}

		switch w.Condition {
		case "changed", "above", "below", "equals", "contains", "regex", "absence":
		default:
			return fmt.Errorf("watch %q: unknown condition %q", w.ID, w.Condition)
		}

		if w.Type == "json" && w.JSONPath == "" {
			return fmt.Errorf("watch %q: json_path is required for type=json", w.ID)
		}
		if w.Type == "html" && w.CSSSelector == "" {
			return fmt.Errorf("watch %q: css_selector is required for type=html", w.ID)
		}
		if w.Condition == "regex" && w.Pattern == "" {
			return fmt.Errorf("watch %q: pattern is required for condition=regex", w.ID)
		}
		if (w.Condition == "above" || w.Condition == "below") && w.Threshold == "" {
			return fmt.Errorf("watch %q: threshold is required for condition=%s", w.ID, w.Condition)
		}
	}
	return nil
}
