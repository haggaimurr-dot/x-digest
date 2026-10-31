package main

import (
	"errors"
	"net/http"

	"xdigest/internal/translate"
)

// ---- translation ----
// POST /api/v1/translate {texts: string[], target: "en"|"zh-CN"} ->
// {translations: string[]} in the same order. Untranslated items come back
// as the original text; the endpoint never fails the whole request because
// of one item. Without TRANSLATE_API_KEY it answers 503.

func (s *server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	var req struct {
		Texts  []string `json:"texts"`
		Target string   `json:"target"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if !translate.ValidTarget(req.Target) {
		writeErr(w, 400, "INVALID_TARGET", "target must be \"en\" or \"zh-CN\"")
		return
	}
	if len(req.Texts) > 20 {
		writeErr(w, 400, "TOO_MANY_TEXTS", "at most 20 texts per request")
		return
	}
	out, err := s.tr.Translate(r.Context(), req.Texts, req.Target)
	if errors.Is(err, translate.ErrNotConfigured) {
		writeErr(w, 503, "TRANSLATE_UNAVAILABLE", "translation is not configured on this server")
		return
	}
	if err != nil {
		writeErr(w, 500, "TRANSLATE_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"translations": out})
}
