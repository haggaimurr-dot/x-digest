package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Settings cloud sync (2026-10-28).
//
//	GET /api/v1/settings → {settings, updated_at} (404 when never saved)
//	PUT /api/v1/settings {settings: {...}, updated_at: RFC3339}
//	    → 200 saved; 409 {settings, updated_at} when the server copy is newer
//	      (last writer by updated_at wins; the page then applies the server
//	      copy).
//
// Settings belong to the API token: the row key is sha256(token), so every
// device that holds the token shares one copy. Unlike other reads, GET
// needs the token too (it is how the copy is found). Without API_TOKEN on
// the server one shared "default" copy is used.

type settingsStore interface {
	GetUserSettings(key string) (json.RawMessage, time.Time, error)
	PutUserSettings(key string, settings json.RawMessage, updatedAt time.Time) error
}

const maxSettingsBytes = 16 << 10

func settingsKey(token string) string {
	if token == "" {
		return "default"
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func requestToken(r *http.Request) string {
	if t := strings.TrimSpace(r.Header.Get("X-API-Token")); t != "" {
		return t
	}
	t, _ := strings.CutPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ")
	return strings.TrimSpace(t)
}

func handleSettings(st settingsStore, apiToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := requestToken(r)
		if apiToken != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(apiToken)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="xdigest"`)
			writeErr(w, 401, "UNAUTHORIZED", "settings sync needs the API token")
			return
		}
		key := settingsKey(apiToken)
		switch r.Method {
		case http.MethodGet:
			s, at, err := st.GetUserSettings(key)
			if errors.Is(err, sql.ErrNoRows) {
				writeErr(w, 404, "NOT_FOUND", "no settings saved yet")
				return
			}
			if err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"settings": s, "updated_at": at.UTC().Format(time.RFC3339Nano)})
		case http.MethodPut:
			var body struct {
				Settings  json.RawMessage `json:"settings"`
				UpdatedAt string          `json:"updated_at"`
			}
			if !decodeBody(w, r, &body) {
				return
			}
			s := bytes.TrimSpace(body.Settings)
			at, err := time.Parse(time.RFC3339Nano, body.UpdatedAt)
			switch {
			case len(s) == 0 || s[0] != '{':
				writeErr(w, 422, "VALIDATION", "settings must be a JSON object")
				return
			case len(s) > maxSettingsBytes:
				writeErr(w, 422, "VALIDATION", "settings too large (16KB max)")
				return
			case err != nil:
				writeErr(w, 422, "VALIDATION", "updated_at must be RFC3339")
				return
			}
			cur, curAt, err := st.GetUserSettings(key)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			if err == nil && curAt.After(at) {
				writeJSON(w, 409, map[string]any{"error": "STALE", "settings": cur, "updated_at": curAt.UTC().Format(time.RFC3339Nano)})
				return
			}
			if err := st.PutUserSettings(key, s, at); err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true, "updated_at": at.UTC().Format(time.RFC3339Nano)})
		default:
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PUT")
		}
	}
}
