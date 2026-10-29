// Command api serves the x-digest Go backend (see ../SPEC.md contract).
//
// Environment:
//
//	XDIGEST_ADDR           listen address, default 127.0.0.1:18090
//	XDIGEST_DSN            MySQL DSN, default "xdigest:CHANGE_ME@tcp(127.0.0.1:3306)/xdigest"
//	XDIGEST_ATTACHMENT_DIR disk directory for attachment files,
//	                       default /var/lib/xdigest/attachments
//	GITHUB_TOKEN            optional GitHub personal access token for repo
//	                       sync (higher rate limits, private repos).
//	                       Unset = unauthenticated mode.
//	TRANSLATE_API_BASE      translation relay base URL,
//	                       default https://api.apikey.fan
//	TRANSLATE_API_KEY       translation relay API key. Unset = POST
//	                       /api/v1/translate answers 503; the UI language
//	                       toggle still works (UI strings are local).
//	BILIBILI_SESSDATA       optional logged-in Bilibili cookie (AI subtitles
//	                       for summaries, fewer risk-control blocks).
//	BILI_HTTP_PROXY         optional proxy for Bilibili requests.
//	TRANSLATE_MODEL         translation chat model, default deepseek-flash.
//	                       The same relay/key/model also write the
//	                       YouTube AI summaries (see yt_summary.go).
//	GROK_API_KEY            relay key of the Grok group: X account refresh
//	                       (x_import.go; also GROK_MODEL, X_HTTP_PROXY).
//	API_TOKEN               bearer token required for every write request
//	                       (auth.go). Unset = no check.
//	AI_DAILY_LIMIT          max AI-spending requests per day (quota.go).
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"xdigest/internal/bilibili"
	"xdigest/internal/github"
	"xdigest/internal/store"
	"xdigest/internal/translate"
	"xdigest/internal/youtube"
)

const maxAttachmentSize = 5 << 20 // 5 MiB per file

var allowedExts = map[string]bool{
	"pdf": true, "doc": true, "docx": true, "xls": true, "xlsx": true,
	"ppt": true, "pptx": true, "txt": true, "md": true, "csv": true,
	"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true,
	"bmp": true, "avif": true, "svg": true, "ico": true,
}

type server struct {
	st        *store.Store
	attachDir string
	gh        *github.Client
	tr        *translate.Service
	yt        *youtube.Client
	sum       *summaryJobs
	repoSync  *repoSyncer
	office    *officeConverter
}

func main() {
	setupLogging()
	addr := envOr("XDIGEST_ADDR", "127.0.0.1:18090")
	dsn := envOr("XDIGEST_DSN", "xdigest:CHANGE_ME@tcp(127.0.0.1:3306)/xdigest")
	attachDir := envOr("XDIGEST_ATTACHMENT_DIR", "/var/lib/xdigest/attachments")
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		log.Fatalf("attachment dir: %v", err)
	}
	st, err := store.New(dsn)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	s := &server{st: st, attachDir: attachDir, gh: github.NewFromEnv(), tr: translate.NewService(translate.NewFromEnv(), st), yt: youtube.New()}
	s.sum = newSummaryJobs(s.yt)
	s.repoSync = s.newRepoSyncer()
	s.office = newOfficeConverter()
	go s.purgeCategoriesLoop()
	go s.cleanAttachmentsLoop()
	if d := repoSyncInterval(); d > 0 {
		go s.scheduleRepoSync(time.NewTicker(d).C)
	}
	mux := http.NewServeMux()
	m := newMetrics()
	translate.CallHook = m.aiCall
	mux.HandleFunc("/api/v1/metrics", m.handler)
	xi := newXImporter(st)
	xi.register(mux)
	xi.resume()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/v1/healthz", s.handleHealth)
	mux.HandleFunc("/api/v1/posts", s.handlePosts)
	mux.HandleFunc("/api/v1/posts/", s.handlePost)
	mux.HandleFunc("/api/v1/watchlist", s.handleWatchlist)
	mux.HandleFunc("/api/v1/watchlist/groups", s.handleGroups)
	mux.HandleFunc("/api/v1/watchlist/groups/", s.handleGroup)
	mux.HandleFunc("/api/v1/watchlist/accounts", s.handleAccounts)
	mux.HandleFunc("/api/v1/watchlist/accounts/bulk", s.handleAccountsBulk)
	mux.HandleFunc("/api/v1/watchlist/accounts/", s.handleAccount)
	mux.HandleFunc("/api/v1/tags", s.handleTags)
	mux.HandleFunc("/api/v1/user-tags", s.handleUserTags)
	mux.HandleFunc("/api/v1/user-tags/", s.handleUserTagSub)
	mux.HandleFunc("/api/v1/categories", s.handleCategories)
	mux.HandleFunc("/api/v1/categories/", s.handleCategory)
	mux.HandleFunc("/api/v1/categories/assignments", s.handleCategoryAssignments)
	mux.HandleFunc("/api/v1/replicas", s.handleReplicas)
	mux.HandleFunc("/api/v1/replicas/", s.handleReplicaSub)
	mux.HandleFunc("/api/v1/repos", s.handleRepos)
	mux.HandleFunc("/api/v1/repos/", s.handleRepoSub)
	mux.HandleFunc("/api/v1/repos/groups", s.handleRepoGroups)
	mux.HandleFunc("/api/v1/repos/groups/", s.handleRepoGroup)
	mux.HandleFunc("/api/v1/repos/sync-all", s.handleRepoSyncAll)
	mux.HandleFunc("/api/v1/yt/channels", s.handleYTChannels)
	mux.HandleFunc("/api/v1/yt/channels/", s.handleYTChannelSub)
	mux.HandleFunc("/api/v1/yt/videos", s.handleYTVideos)
	mux.HandleFunc("/api/v1/yt/videos/", s.handleYTVideoSub)
	// Bilibili uploaders: same routes/JSON as /api/v1/yt (see hub.go).
	(&videoHub{prefix: "bili", idPrefix: "bic_", kinds: map[string]bool{"video": true, "playlist": true},
		st: st.Hub("bili"), pf: bilibili.New(), sum: s.sum}).register(mux)
	mux.HandleFunc("/api/v1/yt/sync", s.handleYTSyncAll)
	mux.HandleFunc("/api/v1/yt/groups", s.handleYTGroups)
	mux.HandleFunc("/api/v1/yt/groups/", s.handleYTGroup)
	mux.HandleFunc("/api/v1/translate", s.handleTranslate)
	mux.HandleFunc("/api/v1/notifications", s.handleNotifications)
	mux.HandleFunc("/api/v1/notifications/", s.handleNotifications)
	mux.HandleFunc("/api/v1/settings", handleSettings(st, os.Getenv("API_TOKEN")))
	mux.HandleFunc("/api/v1/views", s.handleViews)
	mux.HandleFunc("/api/v1/views/", s.handleView)
	mux.HandleFunc("/api/v1/attachments/", s.handleAttachment)
	log.Printf("xdigest-api listening on %s (attachments: %s)", addr, attachDir)
	// auth first (a 401 never counts against the AI quota), then the quota.
	handler := m.observeRequests(requireAuth(os.Getenv("API_TOKEN"), newAIQuota(atoiDefault(os.Getenv("AI_DAILY_LIMIT"), 0)).wrap(mux)))
	log.Fatal(http.ListenAndServe(addr, handler))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, 400, "BAD_JSON", "request body is not valid JSON: "+err.Error())
		return false
	}
	return true
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	if err := s.st.Ping(); err != nil {
		writeErr(w, 503, "DB_UNAVAILABLE", "database ping failed: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
