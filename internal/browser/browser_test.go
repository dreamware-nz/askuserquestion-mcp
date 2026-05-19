package browser

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dreamware-nz/askuserquestion-go"
)

// The Resolver tests drive the real HTTP server through a stub OpenURL
// that races a synthetic browser submission. This exercises the
// full request/render/submit cycle without depending on a real browser.

func sampleReq() askuserquestion.Request {
	return askuserquestion.Request{Questions: []askuserquestion.Question{
		{
			Question: "Auth?",
			Header:   "Auth",
			Options:  []askuserquestion.Option{{Label: "OAuth"}, {Label: "API key"}},
		},
		{
			Question:    "Langs?",
			Header:      "Langs",
			MultiSelect: true,
			Options:     []askuserquestion.Option{{Label: "Go"}, {Label: "Rust"}},
		},
	}}
}

func TestResolverSingleAndMulti(t *testing.T) {
	t.Parallel()
	r := New()
	r.OpenURL = postBack(t, []map[string]any{
		{"selected": []string{"OAuth"}, "other": ""},
		{"selected": []string{"Go", "Rust"}, "other": ""},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	answers, err := r.Ask(ctx, sampleReq())
	if err != nil {
		t.Fatalf("Ask err: %v", err)
	}
	if len(answers) != 2 {
		t.Fatalf("want 2 answers, got %+v", answers)
	}
	if answers[0].Selected[0] != "OAuth" {
		t.Fatalf("q0 = %+v", answers[0])
	}
	if len(answers[1].Selected) != 2 {
		t.Fatalf("q1 selected count = %d", len(answers[1].Selected))
	}
}

func TestResolverOtherOverridesSelected(t *testing.T) {
	t.Parallel()
	r := New()
	r.OpenURL = postBack(t, []map[string]any{
		{"selected": []string{}, "other": "Vincent"},
		{"selected": []string{"Go"}, "other": ""},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	answers, err := r.Ask(ctx, sampleReq())
	if err != nil {
		t.Fatalf("Ask err: %v", err)
	}
	if answers[0].Other != "Vincent" {
		t.Fatalf("expected Other=Vincent, got %+v", answers[0])
	}
}

func TestResolverContextCancel(t *testing.T) {
	t.Parallel()
	r := New()
	r.OpenURL = func(_ string) error { return nil } // never submits

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	_, err := r.Ask(ctx, sampleReq())
	if err == nil {
		t.Fatal("expected ctx error, got nil")
	}
}

func TestResolverFormRenders(t *testing.T) {
	t.Parallel()
	r := New()
	gotHTML := make(chan string, 1)
	r.OpenURL = func(u string) error {
		// fetch the form, capture body, then submit empty payload to
		// release the server.
		go func() {
			resp, err := http.Get(u)
			if err != nil {
				gotHTML <- "ERROR: " + err.Error()
				return
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			gotHTML <- string(b)
			_, _ = http.PostForm(u+"submit", url.Values{
				"payload": {`[{"selected":["OAuth"],"other":""},{"selected":["Go"],"other":""}]`},
			})
		}()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := r.Ask(ctx, sampleReq()); err != nil {
		t.Fatalf("Ask err: %v", err)
	}
	html := <-gotHTML
	for _, frag := range []string{"AskUserQuestion", "Auth", "OAuth", "Langs"} {
		if !strings.Contains(html, frag) {
			t.Fatalf("form missing %q in:\n%s", frag, html)
		}
	}
}

// postBack returns a stub OpenURL that, when invoked, sends a synthetic
// form submission to the resolver's /submit endpoint with the given
// payload. The returned func mirrors the real OpenURL signature so it
// drops into Resolver.OpenURL directly.
func postBack(t *testing.T, payload []map[string]any) func(string) error {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return func(u string) error {
		go func() {
			_, err := http.PostForm(u+"submit", url.Values{"payload": {string(body)}})
			if err != nil {
				t.Logf("synthetic submit failed: %v", err)
			}
		}()
		return nil
	}
}
