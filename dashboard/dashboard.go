package dashboard

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/laohei101/diditchangeyet/config"
	"github.com/laohei101/diditchangeyet/watcher"
)

// Dashboard serves a minimal web UI showing watch statuses.
type Dashboard struct {
	cfg   *config.Config
	state *watcher.StateStore
	log   *slog.Logger
}

func New(cfg *config.Config, state *watcher.StateStore, log *slog.Logger) *Dashboard {
	return &Dashboard{cfg: cfg, state: state, log: log}
}

// Start launches the HTTP server on the given port. Blocks until the server fails.
func (d *Dashboard) Start(port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.handleIndex)
	mux.HandleFunc("/api/status", d.handleStatus)
	mux.HandleFunc("/api/checks/run", d.handleRunCheck)

	addr := fmt.Sprintf(":%d", port)
	d.log.Info("dashboard started", "addr", fmt.Sprintf("http://localhost%s", addr))
	if err := http.ListenAndServe(addr, mux); err != nil {
		d.log.Error("dashboard server error", "error", err)
	}
}

type watchStatus struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Type       string    `json:"type"`
	Condition  string    `json:"condition"`
	Interval   string    `json:"interval"`
	LastValue  string    `json:"last_value"`
	LastCheck  time.Time `json:"last_check"`
	LastError  string    `json:"last_error,omitempty"`
	Triggered  bool      `json:"triggered"`
	CheckCount int       `json:"check_count"`
}

func (d *Dashboard) handleStatus(w http.ResponseWriter, r *http.Request) {
	all := d.state.All()
	statuses := make([]watchStatus, 0, len(d.cfg.Watches))

	for _, wc := range d.cfg.Watches {
		ws := all[wc.ID]
		statuses = append(statuses, watchStatus{
			ID:         wc.ID,
			URL:        wc.URL,
			Type:       wc.Type,
			Condition:  wc.Condition,
			Interval:   wc.Interval,
			LastValue:  ws.LastValue,
			LastCheck:  ws.LastCheck,
			LastError:  ws.LastError,
			Triggered:  ws.Triggered,
			CheckCount: ws.CheckCount,
		})
	}

	sort.Slice(statuses, func(i, j int) bool { return statuses[i].ID < statuses[j].ID })

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(statuses)
}

func (d *Dashboard) handleRunCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query parameter required", http.StatusBadRequest)
		return
	}
	for i := range d.cfg.Watches {
		wc := &d.cfg.Watches[i]
		if wc.ID == id {
			go func() {
				wt, err := watcher.New(wc, d.cfg.GlobalTimeout, d.state, d.log)
				if err != nil {
					d.log.Error("ad-hoc check: build watcher", "id", id, "error", err)
					return
				}
				wt.Run()
			}()
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprintf(w, `{"status":"queued","id":%q}`, id)
			return
		}
	}
	http.Error(w, "watch not found", http.StatusNotFound)
}

var indexTmpl = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>HTTP Change Watcher</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: 'Courier New', monospace; background: #0f0f0f; color: #d4d4d4; padding: 24px; }
  h1 { color: #4ec9b0; font-size: 1.4rem; margin-bottom: 20px; }
  .subtitle { color: #6a9955; font-size: 0.85rem; margin-bottom: 24px; }
  table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
  th { background: #1e1e1e; color: #9cdcfe; padding: 10px 12px; text-align: left;
       border-bottom: 2px solid #333; white-space: nowrap; }
  td { padding: 9px 12px; border-bottom: 1px solid #1e1e1e; vertical-align: top; }
  tr:hover td { background: #1a1a1a; }
  .ok   { color: #4ec9b0; }
  .err  { color: #f44747; }
  .trig { color: #dcdcaa; }
  .url  { color: #9cdcfe; text-decoration: none; max-width: 300px;
          display: inline-block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .badge { display: inline-block; padding: 2px 7px; border-radius: 3px;
           font-size: 0.75rem; font-weight: bold; }
  .badge-ok   { background: #0d3b2e; color: #4ec9b0; }
  .badge-err  { background: #3b0d0d; color: #f44747; }
  .badge-trig { background: #3b3b0d; color: #dcdcaa; }
  .run-btn { background: #264f78; color: #9cdcfe; border: none; padding: 3px 9px;
             border-radius: 3px; cursor: pointer; font-family: inherit; font-size: 0.8rem; }
  .run-btn:hover { background: #37699e; }
  .ts { color: #6a9955; font-size: 0.78rem; }
  #refresh { color: #6a9955; font-size: 0.8rem; margin-top: 12px; }
</style>
</head>
<body>
<h1>HTTP Change Watcher</h1>
<p class="subtitle">Auto-refreshes every 10 seconds &mdash; <span id="next-refresh"></span></p>
<table>
  <thead>
    <tr>
      <th>ID</th><th>URL</th><th>Type</th><th>Condition</th>
      <th>Last Value</th><th>Last Check</th><th>Checks</th><th>Status</th><th>Action</th>
    </tr>
  </thead>
  <tbody id="tbody"><tr><td colspan="9" style="color:#555;text-align:center">Loading…</td></tr></tbody>
</table>
<p id="refresh"></p>

<script>
let countdown = 10;
function statusBadge(w) {
  if (w.last_error) return '<span class="badge badge-err">ERROR</span>';
  if (w.triggered)  return '<span class="badge badge-trig">TRIGGERED</span>';
  if (w.check_count > 0) return '<span class="badge badge-ok">OK</span>';
  return '<span class="badge" style="background:#222;color:#555">PENDING</span>';
}
function fmtTime(ts) {
  if (!ts || ts === '0001-01-01T00:00:00Z') return '—';
  const d = new Date(ts);
  return '<span class="ts">' + d.toLocaleString() + '</span>';
}
function truncate(s, n) {
  return s && s.length > n ? s.substring(0, n) + '…' : (s || '—');
}
function load() {
  fetch('/api/status').then(r => r.json()).then(data => {
    const tb = document.getElementById('tbody');
    if (!data || data.length === 0) {
      tb.innerHTML = '<tr><td colspan="9" style="color:#555;text-align:center">No watches configured</td></tr>';
      return;
    }
    tb.innerHTML = data.map(w => ` + "`" + `<tr>
      <td><strong>${w.id}</strong></td>
      <td><a class="url" href="${w.url}" target="_blank" title="${w.url}">${w.url}</a></td>
      <td>${w.type || '—'}</td>
      <td>${w.condition || '—'}</td>
      <td title="${w.last_value || ''}">${truncate(w.last_value, 40)}</td>
      <td>${fmtTime(w.last_check)}</td>
      <td>${w.check_count}</td>
      <td>${statusBadge(w)}${w.last_error ? '<br><span class="err" style="font-size:0.75rem">'+w.last_error+'</span>' : ''}</td>
      <td><button class="run-btn" onclick="runCheck('${w.id}')">Run now</button></td>
    </tr>` + "`" + `).join('');
    document.getElementById('refresh').textContent =
      'Last loaded: ' + new Date().toLocaleTimeString();
  }).catch(e => {
    document.getElementById('tbody').innerHTML =
      '<tr><td colspan="9" class="err">Failed to load: ' + e + '</td></tr>';
  });
}
function runCheck(id) {
  fetch('/api/checks/run?id=' + encodeURIComponent(id), {method:'POST'})
    .then(r => { if (r.ok) setTimeout(load, 1500); });
}
function tick() {
  countdown--;
  document.getElementById('next-refresh').textContent = 'next refresh in ' + countdown + 's';
  if (countdown <= 0) { countdown = 10; load(); }
}
load();
setInterval(tick, 1000);
</script>
</body>
</html>`))

func (d *Dashboard) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTmpl.Execute(w, nil); err != nil {
		d.log.Error("rendering dashboard", "error", err)
	}
}
