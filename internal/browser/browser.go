// Package browser implements a Resolver that asks the user via an
// ephemeral local HTTP server and a browser-rendered form.
//
// Lifecycle of a single Ask call:
//  1. bind an OS-assigned localhost port (loopback only)
//  2. publish a one-shot HTTP server with two handlers:
//     - GET  /         renders the form for the current request
//     - POST /submit   accepts the user's selections and signals done
//  3. attempt to open the URL in the user's browser via OpenURL
//  4. block until the form is submitted, the user cancels, or ctx is done
//  5. tear down the HTTP server and return the answers
//
// Only one Ask runs at a time per Resolver instance (guarded by a mutex)
// so a misbehaving model issuing parallel tool calls won't race for the
// same port. Tools may call Ask serially as much as they like.
package browser

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dreamware-nz/askuserquestion-go"
)

//go:embed form.html
var assets embed.FS

// Resolver implements server.Resolver by hosting an ephemeral form.
type Resolver struct {
	// Host is the bind address; defaults to 127.0.0.1.
	Host string
	// OpenURL is the function used to launch the user's browser. Defaults
	// to platform-appropriate `open` / `xdg-open` / `rundll32`. Tests can
	// inject a stub.
	OpenURL func(url string) error
	// Logger receives one-line status messages (server start, browser
	// launch, errors). When nil, messages go to io.Discard so an MCP
	// server using stdio doesn't get polluted output.
	Logger io.Writer

	mu sync.Mutex
}

// New returns a Resolver with defaults appropriate for stdio MCP usage:
// loopback binding, platform-appropriate browser launcher, no logging.
func New() *Resolver {
	return &Resolver{
		Host:    "127.0.0.1",
		OpenURL: openBrowser,
	}
}

// Ask implements server.Resolver.
func (r *Resolver) Ask(ctx context.Context, req askuserquestion.Request) ([]askuserquestion.Answer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	host := r.Host
	if host == "" {
		host = "127.0.0.1"
	}

	listener, err := net.Listen("tcp", host+":0")
	if err != nil {
		return nil, fmt.Errorf("browser resolver: bind listener: %w", err)
	}

	type result struct {
		answers []askuserquestion.Answer
		err     error
	}
	out := make(chan result, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		renderForm(w, req)
	})
	mux.HandleFunc("/submit", func(w http.ResponseWriter, httpReq *http.Request) {
		answers, err := parseSubmission(httpReq, req)
		// Always render a thank-you page so the user knows it's safe to
		// close the tab, even when the parse failed. The error path
		// still propagates back to the agent through `out`.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(thanksHTML))
		select {
		case out <- result{answers: answers, err: err}:
		default:
		}
	})

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go func() {
		_ = srv.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	url := fmt.Sprintf("http://%s/", listener.Addr().String())
	r.log("AskUserQuestion: open %s in your browser to answer", url)
	if r.OpenURL != nil {
		if err := r.OpenURL(url); err != nil {
			r.log("AskUserQuestion: could not auto-open browser: %v", err)
		}
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-out:
		return res.answers, res.err
	}
}

func (r *Resolver) log(format string, args ...any) {
	if r.Logger == nil {
		return
	}
	fmt.Fprintf(r.Logger, format+"\n", args...)
}

// renderForm writes the picker form using the embedded template. The
// template intentionally hand-rolls a tiny zero-dep HTML+CSS+JS UI so
// the binary stays self-contained.
func renderForm(w http.ResponseWriter, req askuserquestion.Request) {
	tmpl, err := template.ParseFS(assets, "form.html")
	if err != nil {
		http.Error(w, "template parse: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, req); err != nil {
		http.Error(w, "template exec: "+err.Error(), http.StatusInternalServerError)
	}
}

// parseSubmission turns a posted form into the per-question Answer slice.
// The form posts a single JSON document under the "payload" field; this
// keeps the parsing path explicit and avoids per-field name juggling.
func parseSubmission(httpReq *http.Request, req askuserquestion.Request) ([]askuserquestion.Answer, error) {
	if err := httpReq.ParseForm(); err != nil {
		return nil, fmt.Errorf("parse form: %w", err)
	}
	raw := httpReq.FormValue("payload")
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("empty payload")
	}
	var picks []struct {
		Selected []string `json:"selected"`
		Other    string   `json:"other"`
	}
	if err := json.Unmarshal([]byte(raw), &picks); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	if len(picks) != len(req.Questions) {
		return nil, fmt.Errorf("expected %d answers, got %d", len(req.Questions), len(picks))
	}
	answers := make([]askuserquestion.Answer, len(picks))
	for i, p := range picks {
		answers[i] = askuserquestion.Answer{
			Question: req.Questions[i].Question,
			Selected: p.Selected,
			Other:    p.Other,
		}
	}
	return answers, nil
}

const thanksHTML = `<!doctype html>
<meta charset="utf-8">
<title>Submitted</title>
<style>
body { font-family: system-ui, -apple-system, sans-serif; max-width: 36rem;
  margin: 4rem auto; padding: 0 1.5rem; color: #222; line-height: 1.5; }
h1 { font-size: 1.25rem; margin-bottom: 0.5rem; }
p  { color: #555; }
</style>
<h1>Thanks — answer recorded.</h1>
<p>You can close this tab.</p>
`

// openBrowser launches the system default browser for the given URL.
// Errors are returned so the Resolver can log them; the user can always
// click the URL printed to the logger instead.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
