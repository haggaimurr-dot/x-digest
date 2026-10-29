package main

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"xdigest/internal/translate"
	"xdigest/internal/xpost"
)

// memAuto is an in-memory autoRefreshStore over a fixed watchlist; reads
// (when non-nil) gets a value after every AutoRefreshUsernames call.
type memAuto struct {
	mu    sync.Mutex
	watch map[string]bool
	auto  []string
	reads chan struct{}
}

func (m *memAuto) AutoRefreshUsernames() ([]string, error) {
	m.mu.Lock()
	out := append([]string{}, m.auto...)
	m.mu.Unlock()
	sort.Strings(out)
	if m.reads != nil {
		m.reads <- struct{}{}
	}
	return out, nil
}
func (m *memAuto) SetAutoRefresh(u []string) error { m.auto = append([]string{}, u...); return nil }
func (m *memAuto) AccountExists(u string) bool     { return m.watch[u] }

func TestRefreshAccountsHandler(t *testing.T) {
	st := &memAuto{watch: map[string]bool{"a": true, "b": true}}
	x := &xImporter{auto: st, interval: 6 * time.Hour}
	call := func(method, body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		x.handleRefreshAccounts(rec, httptest.NewRequest(method, "/api/v1/x/refresh/accounts", strings.NewReader(body)))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, out := call("GET", ""); code != 200 || out["interval"] != "6h0m0s" || len(out["usernames"].([]any)) != 0 {
		t.Fatalf("GET = %d %v", code, out)
	}
	if code, out := call("PUT", `{"usernames":["@b"," a ","b",""]}`); code != 200 || !reflect.DeepEqual(out["usernames"], []any{"a", "b"}) {
		t.Fatalf("PUT = %d %v", code, out)
	}
	if code, out := call("PUT", `{"usernames":["a","ghost"]}`); code != 422 || !strings.Contains(out["message"].(string), "ghost") {
		t.Fatalf("PUT unknown = %d %v", code, out)
	}
	if !reflect.DeepEqual(st.auto, []string{"b", "a"}) {
		t.Fatalf("a rejected PUT changed the set: %v", st.auto)
	}
	if code, _ := call("PUT", `{"usernames":[]}`); code != 200 || len(st.auto) != 0 {
		t.Fatalf("clear = %d %v", code, st.auto)
	}
	if code, _ := call("PUT", `nope`); code != 400 {
		t.Fatalf("bad JSON = %d", code)
	}
	if code, _ := call("DELETE", ""); code != 405 {
		t.Fatalf("DELETE = %d", code)
	}
	x.interval = 0
	if _, out := call("GET", ""); out["interval"] != "" {
		t.Fatalf("schedule off: interval = %v", out["interval"])
	}
}

// The scheduled refresh covers only the configured accounts and skips a
// tick when none is configured.
func TestScheduledRefreshUsesConfiguredAccounts(t *testing.T) {
	st := &memAuto{watch: map[string]bool{"a": true, "b": true, "c": true}, reads: make(chan struct{})}
	got := make(chan string, 4)
	x := &xImporter{auto: st, jobs: &memJobs{},
		gk: &xpost.Finder{APIKey: "k"}, llm: &translate.Client{APIKey: "k"}}
	x.doAccount = func(a *refreshAccount) {
		got <- a.Username
		x.set(func() { a.State = "done" })
	}
	tick := make(chan time.Time)
	go x.scheduleRefresh(time.Hour, tick)

	tick <- time.Now() // nothing configured: no job
	<-st.reads
	st.mu.Lock()
	st.auto = []string{"c"}
	st.mu.Unlock()
	tick <- time.Now()
	<-st.reads
	select {
	case u := <-got:
		if u != "c" {
			t.Fatalf("refreshed %q; want c", u)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("scheduled refresh did not start")
	}
	select {
	case u := <-got:
		t.Fatalf("unexpected extra account %q", u)
	case <-time.After(200 * time.Millisecond):
	}
}
