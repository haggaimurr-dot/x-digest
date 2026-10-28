package xpost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Fetcher reads single posts from the public fxtwitter API (no key) and
// turns them into data/raw_posts.json items (README "raw_posts.json 字段"),
// so importer.RawContent and Analyze treat them like bot-pulled posts.
//
//	XPOST_API_BASE   default https://api.fxtwitter.com
//	X_HTTP_PROXY     optional proxy for those requests (servers that
//	                 cannot reach X directly)
type Fetcher struct {
	Base string
	HTTP *http.Client
}

func NewFetcherFromEnv() *Fetcher {
	base := strings.TrimSuffix(os.Getenv("XPOST_API_BASE"), "/")
	if base == "" {
		base = "https://api.fxtwitter.com"
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if p := os.Getenv("X_HTTP_PROXY"); p != "" {
		if u, err := url.Parse(p); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &Fetcher{Base: base, HTTP: &http.Client{Timeout: 30 * time.Second, Transport: tr}}
}

var statusRe = regexp.MustCompile(`(?i)(?:x|twitter|fxtwitter|vxtwitter|fixupx)\.com/(?:([A-Za-z0-9_]{1,15})|i(?:/web)?)/status(?:es)?/(\d+)`)

// ParseStatusURL takes an X post link (x.com / twitter.com / mobile /
// fx mirrors) or a bare numeric id and returns the post id.
func ParseStatusURL(in string) (string, error) {
	in = strings.TrimSpace(in)
	if m := statusRe.FindStringSubmatch(in); m != nil {
		return m[2], nil
	}
	if regexp.MustCompile(`^\d{5,25}$`).MatchString(in) {
		return in, nil
	}
	return "", fmt.Errorf("不是 X 帖子链接：%s", in)
}

type fxAuthor struct {
	ID         string `json:"id"`
	ScreenName string `json:"screen_name"`
	Name       string `json:"name"`
	AvatarURL  string `json:"avatar_url"`
	Desc       string `json:"description"`
}

type fxTweet struct {
	ID               string   `json:"id"`
	URL              string   `json:"url"`
	Text             string   `json:"text"`
	Author           fxAuthor `json:"author"`
	CreatedTimestamp int64    `json:"created_timestamp"`
	Lang             string   `json:"lang"`
	Likes            int64    `json:"likes"`
	Replies          int64    `json:"replies"`
	Retweets         int64    `json:"retweets"`
	Quotes           int64    `json:"quotes"`
	Views            int64    `json:"views"`
	ReplyingTo       string   `json:"replying_to"`
	ReplyingStatus   string   `json:"replying_to_status"`
	Quote            *fxTweet `json:"quote"`
	Media            *struct {
		All []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"all"`
	} `json:"media"`
	Article *struct {
		Title       string `json:"title"`
		PreviewText string `json:"preview_text"`
		Content     *struct {
			Blocks []struct {
				Text string `json:"text"`
			} `json:"blocks"`
		} `json:"content"`
	} `json:"article"`
}

// ErrNotFound means the post does not exist, is private or was deleted.
var ErrNotFound = fmt.Errorf("帖子不存在、已删除或不公开")

func (f *Fetcher) tweet(ctx context.Context, id string) (*fxTweet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.Base+"/status/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "xdigest/1.0")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("拉取帖子失败：%w", err)
	}
	defer resp.Body.Close()
	var body struct {
		Code    int      `json:"code"`
		Message string   `json:"message"`
		Tweet   *fxTweet `json:"tweet"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("拉取帖子失败：HTTP %d", resp.StatusCode)
	}
	if body.Code == 404 || body.Code == 401 || (resp.StatusCode == 404) {
		return nil, ErrNotFound
	}
	if body.Tweet == nil {
		return nil, fmt.Errorf("拉取帖子失败：%d %s", body.Code, body.Message)
	}
	return body.Tweet, nil
}

// fullText is the post text, or the X article (title + body) when the
// post is an article share.
func (t *fxTweet) fullText() string {
	text := t.Text
	if a := t.Article; a != nil && a.Title != "" {
		var b strings.Builder
		b.WriteString("【文章】" + a.Title + "\n")
		if a.Content != nil {
			for _, bl := range a.Content.Blocks {
				if s := strings.TrimSpace(bl.Text); s != "" {
					b.WriteString(s + "\n")
				}
			}
		} else if a.PreviewText != "" {
			b.WriteString(a.PreviewText + "\n")
		}
		article := strings.TrimSpace(b.String())
		// An article share's own text is only its x.com/i/article link.
		if strings.TrimSpace(regexp.MustCompile(`https?://\S+`).ReplaceAllString(text, "")) == "" {
			text = article
		} else {
			text += "\n\n" + article
		}
	}
	return text
}

func ref(kind string, t *fxTweet) map[string]any {
	return map[string]any{
		"type": kind, "id": t.ID, "author_id": t.Author.ID,
		"author_username": t.Author.ScreenName, "author_name": t.Author.Name, "text": t.fullText(),
	}
}

// Fetch returns one post as a raw_posts.json item, with the quoted and
// replied-to posts' text in referenced_tweets.
func (f *Fetcher) Fetch(ctx context.Context, id string) (map[string]any, error) {
	t, err := f.tweet(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Author.ScreenName == "" {
		return nil, fmt.Errorf("拉取帖子失败：缺少作者信息")
	}
	created := time.Unix(t.CreatedTimestamp, 0).UTC().Format(time.RFC3339)
	var refs []any
	if t.Quote != nil && t.Quote.ID != "" {
		refs = append(refs, ref("quoted", t.Quote))
	}
	var replyTo any
	if t.ReplyingStatus != "" {
		replyTo = t.ReplyingStatus
		if p, err := f.tweet(ctx, t.ReplyingStatus); err == nil {
			refs = append(refs, ref("replied_to", p))
		} else {
			refs = append(refs, map[string]any{"type": "replied_to", "id": t.ReplyingStatus, "author_username": t.ReplyingTo})
		}
	}
	media := []any{}
	if t.Media != nil {
		for _, m := range t.Media.All {
			media = append(media, map[string]any{"type": m.Type, "url": m.URL})
		}
	}
	u := t.URL
	if u == "" {
		u = "https://x.com/" + t.Author.ScreenName + "/status/" + t.ID
	}
	raw := map[string]any{
		"id": t.ID, "url": u,
		"author_id": t.Author.ID, "author_username": t.Author.ScreenName, "author_name": t.Author.Name,
		"author_avatar": t.Author.AvatarURL, "author_bio": t.Author.Desc,
		"created_at": created, "lang": t.Lang, "text": t.fullText(),
		"note_tweet": nil, "long_text": nil, "in_reply_to_id": replyTo,
		"referenced_tweets": refs, "media": media,
		"public_metrics": map[string]any{"like": t.Likes, "reply": t.Replies, "repost": t.Retweets, "quote": t.Quotes, "impression": t.Views},
		"fetched_at":     time.Now().UTC().Format(time.RFC3339), "source": "fxtwitter",
	}
	return raw, nil
}
