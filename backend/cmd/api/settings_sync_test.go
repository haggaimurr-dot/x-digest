package main

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type memSettings struct {
	data map[string]json.RawMessage
	at   map[string]time.Time
}

func (m *memSettings) GetUserSettings(key string) (json.RawMessage, time.Time, error) {
	s, ok := m.data[key]
	if !ok {
		return nil, time.Time{}, sql.ErrNoRows
	}
	return s, m.at[key], nil
}

func (m *memSettings) PutUserSettings(key string, s json.RawMessage, at time.Time) error {
	m.data[key], m.at[key] = s, at
	return nil
}

func TestSettingsSync(t *testing.T) {
	st := &memSettings{data: map[string]json.RawMessage{}, at: map[string]time.Time{}}
	h := handleSettings(st, "tok")
	do := func(method, body, token string) (int, string) {
		req := httptest.NewRequest(method, "/api/v1/settings", strings.NewReader(body))
		if token != "" {
			req.Header.Set("X-API-Token", token)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec.Code, rec.Body.String()
	}
	if c, _ := do("GET", "", ""); c != 401 {
		t.Fatalf("GET without token: %d", c)
	}
	if c, _ := do("GET", "", "tok"); c != 404 {
		t.Fatalf("GET before any save: %d", c)
	}
	// Device A saves the light theme.
	if c, b := do("PUT", `{"settings":{"x-digest-theme":"light"},"updated_at":"2026-10-28T10:00:00Z"}`, "tok"); c != 200 {
		t.Fatalf("PUT A: %d %s", c, b)
	}
	// Device B reads it.
	if c, b := do("GET", "", "tok"); c != 200 || !strings.Contains(b, `"x-digest-theme":"light"`) {
		t.Fatalf("GET B: %d %s", c, b)
	}
	// An older write loses: 409 with the newer server copy.
	if c, b := do("PUT", `{"settings":{"x-digest-theme":"dark"},"updated_at":"2026-10-28T09:00:00Z"}`, "tok"); c != 409 || !strings.Contains(b, "light") {
		t.Fatalf("stale PUT: %d %s", c, b)
	}
	for _, bad := range []string{`{"settings":[],"updated_at":"2026-10-28T11:00:00Z"}`, `{"settings":{},"updated_at":"yesterday"}`} {
		if c, _ := do("PUT", bad, "tok"); c != 422 {
			t.Fatalf("bad PUT %s: %d", bad, c)
		}
	}
	if settingsKey("tok") == settingsKey("other") || settingsKey("") != "default" {
		t.Fatal("settingsKey")
	}
}
