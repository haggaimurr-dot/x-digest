package bilibili

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Example from the public WBI documentation (bilibili-API-collect).
func TestSignWBIKnownVector(t *testing.T) {
	key := MixinKey("7cd084941338484aae1ad9425b84077c", "4932caff0ff746eab6f01bf08b70ac45")
	if key != "ea1db124af3c7062474693fa704f4ff8" {
		t.Fatalf("mixin key = %s", key)
	}
	got := signWBI(url.Values{"foo": {"114"}, "bar": {"514"}, "zab": {"1919810"}}, key, 1702204169)
	if got.Get("w_rid") != "8f6f2b5b3d485fe1886cec6a0be8c5d4" {
		t.Fatalf("w_rid = %s", got.Get("w_rid"))
	}
}

func TestParseMid(t *testing.T) {
	for in, want := range map[string]string{
		"946974": "946974", " UID:946974 ": "946974", "uid 2": "2",
		"https://space.bilibili.com/946974?spm_id_from=333": "946974",
	} {
		if got, ok := ParseMid(in); !ok || got != want {
			t.Errorf("ParseMid(%q) = %q,%v", in, got, ok)
		}
	}
	if _, ok := ParseMid("@someone"); ok {
		t.Error("@handle is not a mid")
	}
}

func TestParseLength(t *testing.T) {
	if parseLength("12:34") != 754 || parseLength("1:02:03") != 3723 || parseLength("x") != 0 {
		t.Fatal("parseLength")
	}
}

type fakeBili struct {
	srv        *httptest.Server
	spiCalls   int32
	navCalls   int32
	sawBuvid   atomic.Bool
	arcSigned  atomic.Bool
	subtitleOK bool
}

func newFakeBili(t *testing.T) *fakeBili {
	f := &fakeBili{subtitleOK: true}
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, data any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "0", "data": data})
	}
	mux.HandleFunc("/x/frontend/finger/spi", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.spiCalls, 1)
		ok(w, map[string]string{"b_3": "BUVID3-TEST", "b_4": "B4"})
	})
	mux.HandleFunc("/x/web-interface/nav", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.navCalls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": -101, "data": map[string]any{"wbi_img": map[string]string{
			"img_url": "https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png",
			"sub_url": "https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"}}})
	})
	mux.HandleFunc("/x/web-interface/card", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mid") == "404" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": -404, "message": "啥都木有"})
			return
		}
		ok(w, map[string]any{"card": map[string]string{"mid": r.URL.Query().Get("mid"), "name": "测试UP"}})
	})
	mux.HandleFunc("/x/space/wbi/arc/search", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("buvid3"); err == nil && c.Value == "BUVID3-TEST" {
			f.sawBuvid.Store(true)
		}
		q := r.URL.Query()
		if q.Get("w_rid") != "" && q.Get("wts") != "" && q.Get("dm_img_list") == "[]" {
			f.arcSigned.Store(true)
		}
		created := time.Date(2026, 10, 20, 12, 0, 0, 0, shanghai).Unix()
		ok(w, map[string]any{
			"list": map[string]any{"vlist": []map[string]any{
				{"aid": 11, "bvid": "BV1aa", "title": "新视频", "description": "简介 https://example.com", "created": created, "length": "10:05", "pic": "http://i0.hdslb.com/a.jpg", "author": "测试UP"},
				{"aid": 12, "bvid": "BV1bb", "title": "旧视频", "description": "", "created": created - 30*86400, "length": "1:00:00", "pic": "//i0.hdslb.com/b.jpg", "author": "测试UP"},
			}},
			"page": map[string]int{"count": 2},
		})
	})
	mux.HandleFunc("/x/polymer/web-space/seasons_series_list", func(w http.ResponseWriter, r *http.Request) {
		ok(w, map[string]any{"items_lists": map[string]any{
			"seasons_list": []map[string]any{{"meta": map[string]any{"season_id": 7, "name": "合集A", "description": "", "cover": "http://c.jpg", "total": 12}}},
			"series_list":  []map[string]any{{"meta": map[string]any{"series_id": 9, "name": "系列B", "description": "说明", "cover": "", "total": 3}}},
		}})
	})
	mux.HandleFunc("/x/web-interface/view", func(w http.ResponseWriter, r *http.Request) {
		ok(w, map[string]any{"aid": 11, "bvid": "BV1aa", "cid": 501, "title": "新视频", "desc": "简介", "duration": 605})
	})
	mux.HandleFunc("/x/player/wbi/v2", func(w http.ResponseWriter, r *http.Request) {
		subs := []map[string]any{
			{"lan": "en-US", "subtitle_url": "//" + r.Host + "/sub-en.json", "ai_type": 0},
			{"lan": "ai-zh", "subtitle_url": "//" + r.Host + "/sub-ai.json", "ai_type": 1},
			{"lan": "zh-CN", "subtitle_url": "//" + r.Host + "/sub-zh.json", "ai_type": 0},
		}
		ok(w, map[string]any{"subtitle": map[string]any{"subtitles": subs},
			"view_points": []map[string]any{{"content": "开场", "from": 0, "to": 60}, {"content": "正题", "from": 60, "to": 605}}})
	})
	mux.HandleFunc("/sub-zh.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"body": []map[string]any{{"from": 0.5, "content": "大家好"}, {"from": 3, "content": "今天讲 Go"}}})
	})
	mux.HandleFunc("/x/v2/reply/wbi/main", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("oid") != "11" || r.URL.Query().Get("mode") != "3" {
			t.Errorf("unexpected reply query %s", r.URL.RawQuery)
		}
		ok(w, map[string]any{"replies": []map[string]any{
			{"like": 5, "ctime": 1, "member": map[string]string{"uname": "a"}, "content": map[string]string{"message": "一般"}},
			{"like": 900, "ctime": 2, "member": map[string]string{"uname": "b"}, "content": map[string]string{"message": "最赞"}},
			{"like": 50, "ctime": 3, "member": map[string]string{"uname": "c"}, "content": map[string]string{"message": "  "}},
			{"like": 60, "ctime": 4, "member": map[string]string{"uname": "d"}, "content": map[string]string{"message": "第二"}},
		}})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeBili) client() *Client {
	c := NewWith(&http.Client{Timeout: 5 * time.Second}, f.srv.URL, "")
	// subtitle URLs are protocol-relative to the fake server; httpsURL
	// would make them https, so serve them over plain http in tests.
	c.HTTP.Transport = rewriteHTTPS{}
	return c
}

type rewriteHTTPS struct{}

func (rewriteHTTPS) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme == "https" {
		r = r.Clone(r.Context())
		r.URL.Scheme = "http"
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestResolve(t *testing.T) {
	f := newFakeBili(t)
	c := f.client()
	mid, name, err := c.Resolve(context.Background(), "https://space.bilibili.com/946974")
	if err != nil || mid != "946974" || name != "测试UP" {
		t.Fatalf("got %s %s %v", mid, name, err)
	}
	if _, _, err := c.Resolve(context.Background(), "404"); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("want not-found error, got %v", err)
	}
	if _, _, err := c.Resolve(context.Background(), "@x"); err == nil {
		t.Fatal("want input error")
	}
}

func TestSyncVideosAndCollections(t *testing.T) {
	f := newFakeBili(t)
	c := f.client()
	res, err := c.Sync(context.Background(), "946974", "2026-10-01", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "synced" || res.ChannelName != "测试UP" || res.SyncedTabs != 2 {
		t.Fatalf("result %+v", res)
	}
	if !f.sawBuvid.Load() || !f.arcSigned.Load() {
		t.Errorf("space listing should carry buvid3 and a WBI signature")
	}
	if f.spiCalls != 1 || f.navCalls != 1 {
		t.Errorf("spi/nav should be fetched once each: %d %d", f.spiCalls, f.navCalls)
	}
	var ids []string
	for _, v := range res.Videos {
		ids = append(ids, v.ID+":"+v.Kind)
	}
	// BV1bb is older than date_from and must be dropped.
	if strings.Join(ids, ",") != "BV1aa:video,season-7:playlist,series-9:playlist" {
		t.Fatalf("ids = %v", ids)
	}
	v := res.Videos[0]
	if v.Duration != 605 || v.PublishedAt != "2026-10-20 12:00:00" || v.ThumbnailURL != "https://i0.hdslb.com/a.jpg" || v.URL != "https://www.bilibili.com/video/BV1aa" {
		t.Errorf("video fields %+v", v)
	}
	if res.Videos[1].Description != "共 12 个视频" || !strings.Contains(res.Videos[2].URL, "/lists/9?type=series") {
		t.Errorf("collection fields %+v %+v", res.Videos[1], res.Videos[2])
	}
}

func TestFetchDetailPrefersHumanChineseSubtitle(t *testing.T) {
	f := newFakeBili(t)
	d, err := f.client().FetchDetail(context.Background(), "BV1aa")
	if err != nil {
		t.Fatal(err)
	}
	if d.TranscriptLang != "zh-CN" || d.TranscriptAuto || len(d.Segments) != 2 || d.Segments[1].Text != "今天讲 Go" {
		t.Fatalf("transcript %q auto=%v %+v", d.TranscriptLang, d.TranscriptAuto, d.Segments)
	}
	if len(d.Chapters) != 2 || d.Chapters[1].Start != 60 || d.Duration != 605 {
		t.Fatalf("chapters %+v duration %d", d.Chapters, d.Duration)
	}
}

func TestFetchTopComments(t *testing.T) {
	f := newFakeBili(t)
	got, err := f.client().FetchTopComments(context.Background(), "BV1aa", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Text != "最赞" || got[1].Text != "第二" || got[0].Author != "b" {
		t.Fatalf("got %+v", got)
	}
}

func TestRiskControlErrorIsReadable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x/frontend/finger/spi" {
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":-352,"message":"-352"}`))
	}))
	defer srv.Close()
	c := NewWith(&http.Client{}, srv.URL, "")
	_, _, err := c.Resolve(context.Background(), "1")
	if err == nil || !strings.Contains(err.Error(), "风控") {
		t.Fatalf("got %v", err)
	}
}
