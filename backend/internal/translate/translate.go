// Package translate provides Chinese/English content translation via an
// OpenAI-compatible chat relay (DeepSeek). Standard library only.
//
// Configuration comes only from the environment:
//
//	TRANSLATE_API_BASE  relay base URL, default https://api.apikey.fan
//	TRANSLATE_API_KEY   relay API key; unset = translation unavailable
//	TRANSLATE_MODEL     chat model, default deepseek-flash
//
// The API key is never written to disk, never logged, and never included in
// errors or responses. The relay's chat responses carry both "content" and
// "reasoning_content"; only "content" is ever read (reasoning_content is not
// even decoded).
package translate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultBaseURL = "https://api.apikey.fan"
	defaultModel   = "deepseek-flash"
	requestTimeout = 30 * time.Second

	// maxBatchChars caps the total source characters of one chat call so a
	// single request stays small; larger lists are split into more calls.
	maxBatchChars = 12000
	// maxResponseBytes caps a single upstream response body.
	maxResponseBytes = 16 << 20
)

// Target languages accepted by the translate endpoint.
const (
	TargetEN = "en"
	TargetZh = "zh-CN"
)

// ValidTarget reports whether t is an accepted target language.
func ValidTarget(t string) bool { return t == TargetEN || t == TargetZh }

// ErrNotConfigured is returned when no API key is configured.
var ErrNotConfigured = errors.New("translation is not configured (TRANSLATE_API_KEY is unset)")

// ContainsCJK reports whether s contains any CJK character (ideographs or
// CJK punctuation). Mixed Chinese+English strings therefore count as
// Chinese and are translated when the target is English.
func ContainsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
			(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
			(r >= 0x3000 && r <= 0x303F) { // CJK Symbols and Punctuation
			return true
		}
	}
	return false
}

// NeedsTranslation reports whether s should be sent to the API for target.
// Empty/whitespace-only strings never need translation. A string already in
// the target language is returned as-is (saves API cost).
func NeedsTranslation(s, target string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	if target == TargetEN {
		return ContainsCJK(s)
	}
	return !ContainsCJK(s)
}

// Hash is the cache key for one (source, target) pair: SHA-256 of the
// source text plus the target language.
func Hash(source, target string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + target))
	return hex.EncodeToString(sum[:])
}

// Client is a minimal OpenAI-compatible chat client used only for
// translation.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	APIKey  string // empty = unconfigured
	Model   string
}

// NewFromEnv builds a client reading TRANSLATE_* from the environment.
func NewFromEnv() *Client {
	base := strings.TrimSuffix(os.Getenv("TRANSLATE_API_BASE"), "/")
	if base == "" {
		base = defaultBaseURL
	}
	model := os.Getenv("TRANSLATE_MODEL")
	if model == "" {
		model = defaultModel
	}
	return &Client{
		HTTP:    &http.Client{Timeout: requestTimeout},
		BaseURL: base,
		APIKey:  os.Getenv("TRANSLATE_API_KEY"),
		Model:   model,
	}
}

// Configured reports whether the client has an API key.
func (c *Client) Configured() bool { return c.APIKey != "" }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			// reasoning_content is deliberately NOT decoded: the relay
			// returns it, but only content is the translation.
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func targetName(target string) string {
	if target == TargetEN {
		return "English"
	}
	return "Simplified Chinese"
}

// CallHook, when set, is told about every model call (metrics: AI call and
// failure counts). Set once at start-up.
var CallHook func(model string, err error)

// chat performs one chat completion and returns the assistant's content.
// The API key is sent only in the Authorization header, never in the URL,
// body, logs, or returned errors.
func (c *Client) chat(ctx context.Context, messages []chatMessage) (out string, err error) {
	if CallHook != nil {
		defer func() { CallHook(c.Model, err) }()
	}
	return c.doChat(ctx, messages)
}

func (c *Client) doChat(ctx context.Context, messages []chatMessage) (string, error) {
	body, err := json.Marshal(chatRequest{Model: c.Model, Messages: messages, Temperature: 0})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Do not include the upstream body in the error: it may echo
		// request data and is not needed to diagnose the failure.
		return "", fmt.Errorf("translate upstream HTTP %d", resp.StatusCode)
	}
	var out chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("translate upstream response: %w", err)
	}
	if out.Error != nil {
		return "", errors.New("translate upstream error")
	}
	if len(out.Choices) == 0 {
		return "", errors.New("translate upstream returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}

// Complete runs one system + user chat completion and returns the
// trimmed content. It reuses the relay, key and model configured for
// translation (the YouTube AI summary calls it).
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}
	content, err := c.chat(ctx, []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(content), nil
}

// translateOne translates a single text with one chat call.
func (c *Client) translateOne(ctx context.Context, text, target string) (string, error) {
	content, err := c.chat(ctx, []chatMessage{
		{Role: "system", Content: "You are a professional translator. Translate the user's text to " + targetName(target) + ". Output ONLY the translation. Preserve formatting and line breaks. Do not add explanations, quotation marks, or notes."},
		{Role: "user", Content: text},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(content), nil
}

// translateBatch translates several texts in one chat call. The request and
// the expected response are both JSON arrays, so newlines inside a text
// cannot confuse item boundaries.
func (c *Client) translateBatch(ctx context.Context, texts []string, target string) ([]string, error) {
	items, err := json.Marshal(texts)
	if err != nil {
		return nil, err
	}
	content, err := c.chat(ctx, []chatMessage{
		{Role: "system", Content: "You are a professional translator. Translate each item of the user's JSON array to " + targetName(target) + ". Return ONLY a JSON array of translated strings, in the same order. Do not add explanations."},
		{Role: "user", Content: string(items)},
	})
	if err != nil {
		return nil, err
	}
	arr, err := parseJSONArray(content)
	if err != nil {
		return nil, fmt.Errorf("translate batch output: %w", err)
	}
	if len(arr) != len(texts) {
		return nil, fmt.Errorf("translate batch length mismatch: got %d, want %d", len(arr), len(texts))
	}
	out := make([]string, len(arr))
	for i, s := range arr {
		out[i] = strings.TrimSpace(s)
	}
	return out, nil
}

// parseJSONArray extracts the first [...] JSON array from model output,
// tolerating markdown fences around it.
func parseJSONArray(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end < 0 || end <= start {
		return nil, errors.New("no JSON array in translation output")
	}
	var arr []string
	if err := json.Unmarshal([]byte(s[start:end+1]), &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// Result is one translated item. Translated is false when the item was
// returned as-is (already in the target language, empty, or failed) —
// callers must not cache those as translations.
type Result struct {
	Text       string
	Translated bool
}

// Translate translates texts to target, preserving order and length. Items
// that are empty or already in the target language are returned as-is
// without an API call. A failed batch falls back to one call per text, and
// a failed single text falls back to the original text for that item only —
// the whole request never fails because of one item.
func (c *Client) Translate(ctx context.Context, texts []string, target string) []Result {
	out := make([]Result, len(texts))
	var todo []int
	for i, s := range texts {
		out[i] = Result{Text: s}
		if NeedsTranslation(s, target) {
			todo = append(todo, i)
		}
	}
	if len(todo) == 0 {
		return out
	}
	for _, group := range groupIndexes(texts, todo) {
		batch := make([]string, len(group))
		for j, idx := range group {
			batch[j] = texts[idx]
		}
		translated, err := c.translateBatch(ctx, batch, target)
		if err != nil {
			for j, idx := range group {
				if s, err := c.translateOne(ctx, batch[j], target); err == nil {
					out[idx] = Result{Text: s, Translated: true}
				}
			}
			continue
		}
		for j, idx := range group {
			out[idx] = Result{Text: translated[j], Translated: true}
		}
	}
	return out
}

// groupIndexes splits the indexes needing translation into groups of at
// most 20 items and maxBatchChars total source characters.
func groupIndexes(texts []string, todo []int) [][]int {
	var groups [][]int
	var cur []int
	chars := 0
	for _, idx := range todo {
		n := utf8.RuneCountInString(texts[idx])
		if len(cur) > 0 && (len(cur) >= 20 || chars+n > maxBatchChars) {
			groups = append(groups, cur)
			cur = nil
			chars = 0
		}
		cur = append(cur, idx)
		chars += n
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	return groups
}

// Cache is the translation cache backing store. Implementations must be
// safe for the handler's use; cache errors are logged and treated as a
// miss, never fatal.
type Cache interface {
	GetTranslations(hashes []string, target string) (map[string]string, error)
	PutTranslations(target string, items map[string]string) error
}

// Service orchestrates short-circuiting, the cache, and the relay client.
type Service struct {
	Client *Client
	Cache  Cache
}

// NewService builds a Service. Client or Cache may be nil only in tests;
// production always passes both.
func NewService(c *Client, cache Cache) *Service {
	return &Service{Client: c, Cache: cache}
}

// Translate translates texts to target in order. It returns
// ErrNotConfigured when no API key is configured, and an error only for an
// invalid target — cache or relay failures degrade to the original text.
func (s *Service) Translate(ctx context.Context, texts []string, target string) ([]string, error) {
	if !ValidTarget(target) {
		return nil, fmt.Errorf("invalid target language: %q", target)
	}
	if s.Client == nil || !s.Client.Configured() {
		return nil, ErrNotConfigured
	}
	out := make([]string, len(texts))
	var miss []int
	var missHashes []string
	for i, src := range texts {
		out[i] = src
		if !NeedsTranslation(src, target) {
			continue
		}
		miss = append(miss, i)
		missHashes = append(missHashes, Hash(src, target))
	}
	if len(miss) == 0 {
		return out, nil
	}
	cached := map[string]string{}
	if s.Cache != nil {
		if got, err := s.Cache.GetTranslations(missHashes, target); err == nil {
			cached = got
		} else {
			log.Printf("translate cache read failed: %v", err)
		}
	}
	var uncached []int
	var uncachedTexts []string
	for j, i := range miss {
		if tr, ok := cached[missHashes[j]]; ok {
			out[i] = tr
		} else {
			uncached = append(uncached, i)
			uncachedTexts = append(uncachedTexts, texts[i])
		}
	}
	// One line per request: how many texts the cache answered (hit rate).
	log.Printf("translate %s: %d texts, %d cache hit, %d to relay", target, len(miss), len(miss)-len(uncached), len(uncached))
	if len(uncached) == 0 {
		return out, nil
	}
	results := s.Client.Translate(ctx, uncachedTexts, target)
	toStore := map[string]string{}
	for j, i := range uncached {
		out[i] = results[j].Text
		if results[j].Translated {
			toStore[uncachedTexts[j]] = results[j].Text
		}
	}
	if len(toStore) > 0 && s.Cache != nil {
		if err := s.Cache.PutTranslations(target, toStore); err != nil {
			log.Printf("translate cache write failed: %v", err)
		}
	}
	return out, nil
}
