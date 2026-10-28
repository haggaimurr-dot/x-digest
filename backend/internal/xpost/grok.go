package xpost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"xdigest/internal/translate"
)

// Finder lists an account's recent post ids with Grok's x_search tool
// through the relay's /v1/responses endpoint. Grok only supplies ids; the
// post itself always comes from the Fetcher, which also checks the author,
// so a made-up id can never become a post.
//
//	GROK_API_KEY    relay key of the group that serves grok-* models
//	                (unset = account refresh is unavailable)
//	GROK_API_BASE   default TRANSLATE_API_BASE, else https://api.apikey.fan
//	GROK_MODEL      default grok-4.3
type Finder struct {
	Base   string
	APIKey string
	Model  string
	HTTP   *http.Client
}

func NewFinderFromEnv() *Finder {
	base := os.Getenv("GROK_API_BASE")
	if base == "" {
		base = os.Getenv("TRANSLATE_API_BASE")
	}
	if base == "" {
		base = "https://api.apikey.fan"
	}
	model := os.Getenv("GROK_MODEL")
	if model == "" {
		model = "grok-4.3"
	}
	return &Finder{Base: strings.TrimSuffix(base, "/"), APIKey: os.Getenv("GROK_API_KEY"), Model: model,
		HTTP: &http.Client{Timeout: 4 * time.Minute}}
}

func (f *Finder) Configured() bool { return f.APIKey != "" }

var idListRe = regexp.MustCompile(`\d{10,25}`)

// RecentIDs returns ids of posts by handle published on or after since
// (UTC date granularity), newest first as Grok reports them.
func (f *Finder) RecentIDs(ctx context.Context, handle string, since time.Time) ([]string, error) {
	from := since.UTC().Format("2006-01-02")
	prompt := fmt.Sprintf(`Use x_search to list every post written by @%[1]s published on or after %[2]s: original posts, quote posts and replies (not reposts of other people's posts). Search thoroughly, up to 60 posts, newest first.
Output ONLY a JSON array of status ids as strings, e.g. ["1234567890123456789"]. Take each id from the post URL (x.com/%[1]s/status/<id>). Output [] if there are none.`, handle, from)
	body, _ := json.Marshal(map[string]any{
		"model": f.Model,
		"input": prompt,
		"tools": []any{map[string]any{"type": "x_search", "allowed_x_handles": []string{handle}, "from_date": from}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.Base+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+f.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("grok upstream HTTP %d", resp.StatusCode)
	}
	var out struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("grok reply: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("grok: %s", out.Error.Message)
	}
	var text strings.Builder
	for _, o := range out.Output {
		if o.Type == "message" {
			for _, c := range o.Content {
				text.WriteString(c.Text)
			}
		}
	}
	return parseIDList(text.String()), nil
}

// parseIDList reads the ids from the reply's first [...] (or, failing
// that, any long digit runs), de-duplicated in order.
func parseIDList(s string) []string {
	if i, j := strings.Index(s, "["), strings.LastIndex(s, "]"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	var ids []string
	seen := map[string]bool{}
	for _, id := range idListRe.FindAllString(s, -1) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// NewGrokChatFromEnv is the fallback model for Analyze: the Grok group of
// the relay (GROK_API_KEY), used when the primary model refuses a post
// (the relay answers 403 for some topics) or fails. nil without a key.
// GROK_DIGEST_MODEL overrides the model (default grok-4.20-0309-non-reasoning).
func NewGrokChatFromEnv() *translate.Client {
	f := NewFinderFromEnv()
	if !f.Configured() {
		return nil
	}
	model := os.Getenv("GROK_DIGEST_MODEL")
	if model == "" {
		model = "grok-4.20-0309-non-reasoning"
	}
	return &translate.Client{BaseURL: f.Base, APIKey: f.APIKey, Model: model, HTTP: &http.Client{Timeout: 3 * time.Minute}}
}
