package store

// HubStore is the video-platform store keyed by a table prefix: "yt" reads
// and writes yt_groups / yt_channels / yt_videos / yt_video_summaries,
// "bili" the bili_* tables of the same shape (2026-10-27). YouTube moved
// onto it on 2026-10-29; the old yt-only store functions are gone.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"xdigest/internal/model"
)

type HubStore struct {
	DB *sql.DB
	P  string // table prefix, e.g. "bili"
}

// Hub returns the store for one platform prefix.
func (s *Store) Hub(prefix string) *HubStore { return &HubStore{DB: s.DB, P: prefix} }

// q replaces the {p} placeholder with the table prefix.
func (h *HubStore) q(sql string) string { return strings.ReplaceAll(sql, "{p}", h.P) }

func (h *HubStore) channelCols() string {
	return h.q(`SELECT c.id,c.channel_id,c.handle,c.name,c.group_id,
	DATE_FORMAT(c.date_from,'%Y-%m-%d'),DATE_FORMAT(c.date_to,'%Y-%m-%d'),
	DATE_FORMAT(c.created_at,'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(c.last_synced_at,'%Y-%m-%d %H:%i:%s'),
	c.sync_status,COALESCE(c.sync_error,''),
	(SELECT COUNT(*) FROM {p}_videos v WHERE v.channel_id=c.channel_id) AS video_count,c.sync_mode
	FROM {p}_channels c `)
}

type rowScanner interface{ Scan(dest ...any) error }

func scanHubChannel(r rowScanner) (model.YTChannel, error) {
	var c model.YTChannel
	var groupID, lastSyncedAt, dateFrom, dateTo sql.NullString
	var videoCount int
	err := r.Scan(&c.ID, &c.ChannelID, &c.Handle, &c.Name, &groupID, &dateFrom, &dateTo,
		&c.CreatedAt, &lastSyncedAt, &c.SyncStatus, &c.SyncError, &videoCount, &c.SyncMode)
	if err != nil {
		return c, err
	}
	scanYTChannel(&c, groupID, lastSyncedAt, dateFrom, dateTo, videoCount)
	return c, nil
}

func (h *HubStore) ListChannels() ([]model.YTChannel, error) {
	rows, err := h.DB.Query(h.channelCols() + "ORDER BY c.created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.YTChannel{}
	for rows.Next() {
		c, err := scanHubChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (h *HubStore) GetChannel(id string) (model.YTChannel, error) {
	return scanHubChannel(h.DB.QueryRow(h.channelCols()+"WHERE c.id=?", id))
}

func (h *HubStore) GetChannelByChannelID(channelID string) (model.YTChannel, error) {
	return scanHubChannel(h.DB.QueryRow(h.channelCols()+"WHERE c.channel_id=?", channelID))
}

func (h *HubStore) ChannelExists(id string) bool {
	var n int
	_ = h.DB.QueryRow(h.q("SELECT COUNT(*) FROM {p}_channels WHERE id=?"), id).Scan(&n)
	return n > 0
}

// CreateChannel inserts a channel row; an empty SyncMode means "full".
// Callers check for duplicates first.
func (h *HubStore) CreateChannel(c model.YTChannel) error {
	mode := c.SyncMode
	if mode == "" {
		mode = "full"
	}
	_, err := h.DB.Exec(h.q(`INSERT INTO {p}_channels(id,channel_id,handle,name,group_id,date_from,date_to,
		created_at,last_synced_at,sync_status,sync_error,sync_mode) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`),
		c.ID, c.ChannelID, c.Handle, c.Name, groupIDArg(c.GroupID), dateArg(c.DateFrom), dateArg(c.DateTo),
		c.CreatedAt, nil, c.SyncStatus, c.SyncError, mode)
	return err
}

// dateArg maps a nil *string to NULL so an unset date means "no limit".
func dateArg(d *string) any {
	if d == nil {
		return nil
	}
	return *d
}

func (h *HubStore) UpdateChannel(id, groupID string, dateFrom, dateTo *string) error {
	_, err := h.DB.Exec(h.q("UPDATE {p}_channels SET group_id=?,date_from=?,date_to=? WHERE id=?"),
		groupIDArg(groupID), dateArg(dateFrom), dateArg(dateTo), id)
	return err
}

// DeleteChannel deletes the channel with its videos, summaries and
// comments in one transaction.
func (h *HubStore) DeleteChannel(id string) error {
	tx, err := h.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var channelID string
	if err := tx.QueryRow(h.q("SELECT channel_id FROM {p}_channels WHERE id=?"), id).Scan(&channelID); err != nil {
		return err
	}
	for _, stmt := range []string{
		"DELETE s FROM {p}_video_summaries s JOIN {p}_videos v ON v.id=s.video_id WHERE v.channel_id=?",
		"DELETE FROM {p}_videos WHERE channel_id=?",
	} {
		if _, err := tx.Exec(h.q(stmt), channelID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(h.q("DELETE FROM {p}_channels WHERE id=?"), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *HubStore) SetChannelSyncStatus(id, status, syncErr string) error {
	_, err := h.DB.Exec(h.q("UPDATE {p}_channels SET last_synced_at=?,sync_status=?,sync_error=? WHERE id=?"),
		Now(), status, syncErr, id)
	return err
}

func (h *HubStore) UpdateChannelMeta(id, name, status, syncErr string) error {
	_, err := h.DB.Exec(h.q("UPDATE {p}_channels SET name=?,last_synced_at=?,sync_status=?,sync_error=? WHERE id=?"),
		name, Now(), status, syncErr, id)
	return err
}

// UpsertVideos inserts new videos and updates re-fetched ones (the id
// PRIMARY KEY makes a re-sync an update, never a duplicate). A re-fetch
// never clobbers a known value with an empty one: description,
// published_at, duration and live_status only change when the incoming
// value is non-empty. Title, thumbnail, URL and kind come from the listing
// and are always overwritten. Empty PublishedAt → NULL, empty Kind → video.
func (h *HubStore) UpsertVideos(channelID string, videos []model.YTVideo) error {
	if len(videos) == 0 {
		return nil
	}
	tx, err := h.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(h.q(`INSERT INTO {p}_videos(id,channel_id,kind,title,description,
		published_at,duration,live_status,thumbnail_url,url,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE kind=VALUES(kind),title=VALUES(title),
		description=IF(VALUES(description)<>'',VALUES(description),description),
		published_at=IF(VALUES(published_at) IS NOT NULL,VALUES(published_at),published_at),
		duration=IF(VALUES(duration) IS NOT NULL,VALUES(duration),duration),
		live_status=IF(VALUES(live_status)<>'',VALUES(live_status),live_status),
		thumbnail_url=VALUES(thumbnail_url),url=VALUES(url)`))
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, v := range videos {
		kind := v.Kind
		if kind == "" {
			kind = "video"
		}
		var publishedAt, duration any
		if v.PublishedAt != "" {
			publishedAt = v.PublishedAt
		}
		if v.Duration > 0 {
			duration = v.Duration
		}
		if _, err := stmt.Exec(v.ID, channelID, kind, v.Title, v.Description,
			publishedAt, duration, v.LiveStatus, v.ThumbnailURL, v.URL, v.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PruneVideosOutsideRange deletes a channel's non-playlist rows published
// outside [from, to] (to includes the whole day; "" = no bound, both "" =
// no-op). Playlists have no date and are kept; so are rows with no date yet
// (the next sync backfills them) and videos added by URL.
func (h *HubStore) PruneVideosOutsideRange(channelID, from, to string) error {
	if from == "" && to == "" {
		return nil
	}
	var outside []string
	args := []any{channelID}
	if from != "" {
		outside = append(outside, "published_at<?")
		args = append(args, from)
	}
	if to != "" {
		outside = append(outside, "published_at>=DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, to)
	}
	_, err := h.DB.Exec(h.q(`DELETE FROM {p}_videos WHERE channel_id=? AND kind<>'playlist'
		AND added_manually=0 AND published_at IS NOT NULL AND (`+strings.Join(outside, " OR ")+")"), args...)
	return err
}

func hubVideoOrder(sort string) string {
	switch sort {
	case "imported_asc":
		return "v.created_at ASC, v.id ASC"
	case "published_desc":
		return "v.published_at DESC, v.id DESC"
	case "published_asc":
		return "v.published_at ASC, v.id ASC"
	default: // "imported_desc": newest imports first
		return "v.created_at DESC, v.id DESC"
	}
}

func (h *HubStore) ListVideos(f YTVideoFilter) ([]model.YTVideo, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var conds []string
	var args []any
	if f.ChannelID != "" {
		conds, args = append(conds, "v.channel_id=?"), append(args, f.ChannelID)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + q + "%"
		conds, args = append(conds, "(v.title LIKE ? OR v.description LIKE ?)"), append(args, like, like)
	}
	if f.Kind != "" {
		conds, args = append(conds, "v.kind=?"), append(args, f.Kind)
	}
	if f.DateFrom != "" {
		conds, args = append(conds, "v.published_at>=?"), append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		conds, args = append(conds, "v.published_at<DATE_ADD(?, INTERVAL 1 DAY)"), append(args, f.DateTo)
	}
	if f.WatchLater {
		conds = append(conds, "v.watch_later=1")
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	rows, err := h.DB.Query(h.q(`SELECT v.id,v.channel_id,COALESCE(c.name,''),v.kind,v.title,COALESCE(v.description,''),
		DATE_FORMAT(v.published_at,'%Y-%m-%d %H:%i:%s'),v.duration,v.live_status,v.thumbnail_url,v.url,
		DATE_FORMAT(v.created_at,'%Y-%m-%d %H:%i:%s'),
		COALESCE(DATE_FORMAT(v.watched_at,'%Y-%m-%d %H:%i:%s'),''),v.watch_later
		FROM {p}_videos v LEFT JOIN {p}_channels c ON c.channel_id=v.channel_id `+
		where+` ORDER BY `+hubVideoOrder(f.Sort)+` LIMIT ?`), append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.YTVideo{}
	for rows.Next() {
		var v model.YTVideo
		var publishedAt sql.NullString
		var duration sql.NullInt64
		if err := rows.Scan(&v.ID, &v.ChannelID, &v.ChannelName, &v.Kind, &v.Title, &v.Description,
			&publishedAt, &duration, &v.LiveStatus, &v.ThumbnailURL, &v.URL, &v.CreatedAt,
			&v.WatchedAt, &v.WatchLater); err != nil {
			return nil, err
		}
		v.PublishedAt = publishedAt.String
		v.Duration = int(duration.Int64)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// User tags (bulk load).
	if len(out) > 0 {
		ids := make([]string, len(out))
		for i, v := range out {
			ids[i] = v.ID
		}
		tagsByVideo := h.videoTags(ids)
		for i, v := range out {
			if tags := tagsByVideo[v.ID]; len(tags) > 0 {
				out[i].Tags = tags
			}
		}
	}
	return out, nil
}

// videoTags returns user tags for the given video IDs.
func (h *HubStore) videoTags(ids []string) map[string][]string {
	out := map[string][]string{}
	if len(ids) == 0 {
		return out
	}
	placeholders := strings.Repeat("?,", len(ids)-1) + "?"
	rows, err := h.DB.Query(h.q(`SELECT video_id, tag FROM {p}_video_tags WHERE video_id IN (`+placeholders+`) ORDER BY tag`), stringSliceToAny(ids)...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var vid, tag string
		if err := rows.Scan(&vid, &tag); err != nil {
			continue
		}
		out[vid] = append(out[vid], tag)
	}
	return out
}

// SetVideoTags replaces the user tags for a video; sql.ErrNoRows for an
// unknown video.
func (h *HubStore) SetVideoTags(videoID string, tags []string) error {
	var n int
	if err := h.DB.QueryRow(h.q(`SELECT COUNT(*) FROM {p}_videos WHERE id=?`), videoID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	if _, err := h.DB.Exec(h.q(`DELETE FROM {p}_video_tags WHERE video_id=?`), videoID); err != nil {
		return err
	}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, err := h.DB.Exec(h.q(`INSERT IGNORE INTO {p}_video_tags(video_id, tag) VALUES(?,?)`), videoID, tag); err != nil {
			return err
		}
	}
	return nil
}

func stringSliceToAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func (h *HubStore) GetVideo(id string) (model.YTVideo, error) {
	var v model.YTVideo
	var published sql.NullString
	var duration sql.NullInt64
	err := h.DB.QueryRow(h.q(`SELECT id,channel_id,kind,title,COALESCE(description,''),
		DATE_FORMAT(published_at,'%Y-%m-%d %H:%i:%s'),duration,live_status,thumbnail_url,url
		FROM {p}_videos WHERE id=?`), id).Scan(&v.ID, &v.ChannelID, &v.Kind, &v.Title, &v.Description,
		&published, &duration, &v.LiveStatus, &v.ThumbnailURL, &v.URL)
	v.PublishedAt, v.Duration = published.String, int(duration.Int64)
	return v, err
}

// ---- groups ----

func (h *HubStore) ListGroups() ([]model.Group, error) {
	rows, err := h.DB.Query(h.q("SELECT id,name,fixed FROM {p}_groups ORDER BY id"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Group{}
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Fixed); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (h *HubStore) CreateGroup(name string) (model.Group, error) {
	g := model.Group{ID: NewID("grp_"), Name: name}
	_, err := h.DB.Exec(h.q("INSERT INTO {p}_groups(id,name,fixed) VALUES(?,?,0)"), g.ID, g.Name)
	return g, err
}

// DeleteGroup moves the group's channels to the default group, then
// deletes it; the fixed default group cannot be deleted.
func (h *HubStore) DeleteGroup(id string) (code string, err error) {
	var fixed bool
	if err = h.DB.QueryRow(h.q("SELECT fixed FROM {p}_groups WHERE id=?"), id).Scan(&fixed); err != nil {
		return "NOT_FOUND", err
	}
	if fixed {
		return "GROUP_FIXED", fmt.Errorf("fixed group cannot be deleted")
	}
	tx, err := h.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		"INSERT IGNORE INTO {p}_groups(id,name,fixed) VALUES('other','默认',1)",
		"UPDATE {p}_channels SET group_id='other' WHERE group_id=?",
		"DELETE FROM {p}_groups WHERE id=?",
	} {
		var args []any
		if strings.Contains(stmt, "?") {
			args = []any{id}
		}
		if _, err := tx.Exec(h.q(stmt), args...); err != nil {
			return "", err
		}
	}
	return "", tx.Commit()
}

func (h *HubStore) GroupExists(id string) bool {
	var n int
	_ = h.DB.QueryRow(h.q("SELECT COUNT(*) FROM {p}_groups WHERE id=?"), id).Scan(&n)
	return n > 0
}

// ---- single-mode channels: only the videos added by URL ----

// SetChannelSyncMode switches a channel between "full" and "single".
func (h *HubStore) SetChannelSyncMode(id, mode string) error {
	_, err := h.DB.Exec(h.q("UPDATE {p}_channels SET sync_mode=? WHERE id=?"), mode, id)
	return err
}

// MarkVideoManual flags a video as added by URL (date pruning keeps it).
func (h *HubStore) MarkVideoManual(id string) error {
	_, err := h.DB.Exec(h.q("UPDATE {p}_videos SET added_manually=1 WHERE id=?"), id)
	return err
}

// ManualVideoURLs returns the URLs of a channel's videos added by URL.
func (h *HubStore) ManualVideoURLs(channelID string) ([]string, error) {
	rows, err := h.DB.Query(h.q("SELECT url FROM {p}_videos WHERE channel_id=? AND added_manually=1"), channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---- AI summaries ----

// GetSummary returns the summary row of (video, lang); ok=false when none.
func (h *HubStore) GetSummary(videoID, lang string) (sum model.YTSummary, ok bool, err error) {
	var summary, keyPoints, chapters, errText sql.NullString
	var resources string
	err = h.DB.QueryRow(h.q(`SELECT video_id,lang,status,summary,key_points,chapters,COALESCE(resources,''),source,transcript_lang,
		model,error,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s')
		FROM {p}_video_summaries WHERE video_id=? AND lang=?`), videoID, lang).
		Scan(&sum.VideoID, &sum.Lang, &sum.Status, &summary, &keyPoints, &chapters, &resources, &sum.Source,
			&sum.TranscriptLang, &sum.Model, &errText, &sum.CreatedAt, &sum.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sum, false, nil
	}
	if err != nil {
		return sum, false, err
	}
	sum.Summary, sum.Error = summary.String, errText.String
	sum.KeyPoints, sum.Chapters = []string{}, []model.YTSummaryChapter{}
	if keyPoints.String != "" {
		_ = json.Unmarshal([]byte(keyPoints.String), &sum.KeyPoints)
	}
	if chapters.String != "" {
		_ = json.Unmarshal([]byte(chapters.String), &sum.Chapters)
	}
	sum.Resources = []model.YTSummaryResource{}
	if resources != "" {
		_ = json.Unmarshal([]byte(resources), &sum.Resources)
	}
	return sum, true, nil
}

func (h *HubStore) StartSummary(videoID, lang string) error {
	now := Now()
	_, err := h.DB.Exec(h.q(`INSERT INTO {p}_video_summaries(video_id,lang,status,summary,key_points,chapters,
		source,transcript_lang,model,error,created_at,updated_at) VALUES(?,?,'pending','','[]','[]','','','','',?,?)
		ON DUPLICATE KEY UPDATE status='pending',error='',updated_at=VALUES(updated_at)`), videoID, lang, now, now)
	return err
}

func (h *HubStore) FinishSummary(sum model.YTSummary) error {
	kp, err := json.Marshal(sum.KeyPoints)
	if err != nil {
		return err
	}
	ch, err := json.Marshal(sum.Chapters)
	if err != nil {
		return err
	}
	resources := sum.Resources
	if resources == nil {
		resources = []model.YTSummaryResource{}
	}
	rs, err := json.Marshal(resources)
	if err != nil {
		return err
	}
	_, err = h.DB.Exec(h.q(`UPDATE {p}_video_summaries SET status='done',summary=?,key_points=?,chapters=?,resources=?,
		source=?,transcript_lang=?,model=?,error='',updated_at=? WHERE video_id=? AND lang=?`),
		sum.Summary, string(kp), string(ch), string(rs), sum.Source, sum.TranscriptLang, sum.Model, Now(), sum.VideoID, sum.Lang)
	return err
}

func (h *HubStore) FailSummary(videoID, lang, msg string) error {
	_, err := h.DB.Exec(h.q(`UPDATE {p}_video_summaries SET status='failed',error=?,updated_at=?
		WHERE video_id=? AND lang=?`), msg, Now(), videoID, lang)
	return err
}
