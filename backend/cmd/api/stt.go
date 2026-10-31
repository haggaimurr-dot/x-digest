package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

// ---- speech-to-text (local Whisper on the box) ----
//
// POST /api/v1/stt  (multipart form, field "audio")
// Forwards the audio to the local STT service (STT_URL, default
// http://127.0.0.1:18101) and returns {"text": "..."}.
// Auth: requireAuth already gates non-GET methods behind the API token.

func sttURL() string {
	if u := os.Getenv("STT_URL"); u != "" {
		return u
	}
	return "http://127.0.0.1:18101"
}

func (s *server) handleSTT(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB
		writeErr(w, 400, "BAD_REQUEST", "audio file required (max 10MB)")
		return
	}
	f, hdr, err := r.FormFile("audio")
	if err != nil {
		writeErr(w, 400, "BAD_REQUEST", "audio file required")
		return
	}
	defer f.Close()

	// Forward to the local STT service as multipart.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("audio", hdr.Filename)
	if err != nil {
		writeErr(w, 500, "STT_ERROR", err.Error())
		return
	}
	if _, err := io.Copy(fw, f); err != nil {
		writeErr(w, 500, "STT_ERROR", err.Error())
		return
	}
	if lang := r.FormValue("lang"); lang != "" {
		_ = mw.WriteField("lang", lang)
	}
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, sttURL()+"/transcribe", &buf)
	if err != nil {
		writeErr(w, 500, "STT_ERROR", err.Error())
		return
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeErr(w, 502, "STT_UNAVAILABLE", "speech service unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeErr(w, 502, "STT_ERROR", err.Error())
		return
	}
	if resp.StatusCode != http.StatusOK {
		writeErr(w, 502, "STT_ERROR", "speech service returned "+resp.Status)
		return
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		writeErr(w, 502, "STT_ERROR", "bad response from speech service")
		return
	}
	writeJSON(w, 200, map[string]string{"text": out.Text})
}
