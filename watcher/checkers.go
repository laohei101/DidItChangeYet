package watcher

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"

	"github.com/laohei101/diditchangeyet/config"
)

// CheckResult holds the outcome of a single fetch+extract operation.
type CheckResult struct {
	Value      string
	StatusCode int
	Err        error
}

// fetch performs the HTTP request defined by the watch config.
func fetch(w *config.Watch, client *http.Client) ([]byte, int, error) {
	var bodyReader io.Reader
	if w.Body != "" {
		bodyReader = strings.NewReader(w.Body)
	}

	req, err := http.NewRequest(w.Method, w.URL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}

	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}

	return body, resp.StatusCode, nil
}

// Check fetches the watch target and extracts the monitored value.
func Check(w *config.Watch, client *http.Client) CheckResult {
	body, statusCode, err := fetch(w, client)
	if err != nil {
		if w.Condition == "absence" {
			// A fetch failure counts as absent
			return CheckResult{Value: "ABSENT", StatusCode: 0}
		}
		return CheckResult{Err: err}
	}

	switch w.Type {
	case "http_status":
		return CheckResult{
			Value:      fmt.Sprintf("%d", statusCode),
			StatusCode: statusCode,
		}

	case "json":
		result := gjson.GetBytes(body, w.JSONPath)
		if !result.Exists() {
			if w.Condition == "absence" {
				return CheckResult{Value: "ABSENT", StatusCode: statusCode}
			}
			return CheckResult{
				Err: fmt.Errorf("json path %q not found in response", w.JSONPath),
			}
		}
		return CheckResult{Value: result.String(), StatusCode: statusCode}

	case "html":
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			return CheckResult{Err: fmt.Errorf("parsing HTML: %w", err)}
		}
		sel := doc.Find(w.CSSSelector)
		if sel.Length() == 0 {
			if w.Condition == "absence" {
				return CheckResult{Value: "ABSENT", StatusCode: statusCode}
			}
			return CheckResult{
				Err: fmt.Errorf("CSS selector %q matched no elements", w.CSSSelector),
			}
		}
		var value string
		if w.Attribute != "" {
			value, _ = sel.First().Attr(w.Attribute)
		} else {
			value = strings.TrimSpace(sel.First().Text())
		}
		return CheckResult{Value: value, StatusCode: statusCode}

	case "text":
		hash := sha256.Sum256(body)
		return CheckResult{
			Value:      fmt.Sprintf("%x", hash),
			StatusCode: statusCode,
		}

	default:
		return CheckResult{Err: fmt.Errorf("unknown watcher type: %q", w.Type)}
	}
}

// NewHTTPClient creates an HTTP client with the given timeout string (e.g. "30s").
func NewHTTPClient(timeout string) *http.Client {
	d := 30 * time.Second
	if timeout != "" {
		if parsed, err := time.ParseDuration(timeout); err == nil {
			d = parsed
		}
	}
	return &http.Client{
		Timeout: d,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}
}
