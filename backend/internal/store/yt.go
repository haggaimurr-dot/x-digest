package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"xdigest/internal/model"
)

// ---- YouTube channels / videos (prototype 2026-10-26) ----

// YTVideoFilter filters ListYTVideos. DateFrom/DateTo are "2006-01-02"
// calendar dates (Shanghai day bounds, like the post date filter).
// Kind is video|short|live|playlist; "" means all kinds.
// Limit <= 0 means the default 100.
type YTVideoFilter struct {
	ChannelID string
	Q         string
	DateFrom  string
	DateTo    string
	Kind      string
	Limit     int
	// WatchLater: only videos marked 稍后观看.
	WatchLater bool
}

func scanYTChannel(c *model.YTChannel, groupID, lastSyncedAt, dateFrom, dateTo sql.NullString, videoCount int) {
	if groupID.Valid {
		c.GroupID = groupID.String
	}
	if lastSyncedAt.Valid {
		v := lastSyncedAt.String
		c.LastSyncedAt = &v
	}
	if dateFrom.Valid {
		v := dateFrom.String
		c.DateFrom = &v
	}
	if dateTo.Valid {
		v := dateTo.String
		c.DateTo = &v
	}
	c.VideoCount = videoCount
}

// ListYTChannels returns all followed channels, oldest first, each with
// its stored video count.
func (s *Store) ListYTChannels() ([]model.YTChannel, error) {
	rows, err := s.DB.Query(`SELECT c.id,c.channel_id,c.handle,c.name,c.group_id,
		DATE_FORMAT(c.date_from,'%Y-%m-%d'),DATE_FORMAT(c.date_to,'%Y-%m-%d'),
		c.created_at,c.last_synced_at,c.sync_status,c.sync_error,
		(SELECT COUNT(*) FROM yt_videos v WHERE v.channel_id=c.channel_id) AS video_count,c.sync_mode
		FROM yt_channels c ORDER BY c.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.YTChannel{}
	for rows.Next() {
		var c model.YTChannel
		var groupID, lastSyncedAt, dateFrom, dateTo sql.NullString
		var videoCount int
		if err := rows.Scan(&c.ID, &c.ChannelID, &c.Handle, &c.Name, &groupID,
			&dateFrom, &dateTo,
			&c.CreatedAt, &lastSyncedAt, &c.SyncStatus, &c.SyncError, &videoCount, &c.SyncMode); err != nil {
			return nil, err
		}
		scanYTChannel(&c, groupID, lastSyncedAt, dateFrom, dateTo, videoCount)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) scanYTChannelRow(row *sql.Row) (model.YTChannel, error) {
	var c model.YTChannel
	var groupID, lastSyncedAt, dateFrom, dateTo sql.NullString
	var videoCount int
	err := row.Scan(&c.ID, &c.ChannelID, &c.Handle, &c.Name, &groupID,
		&dateFrom, &dateTo,
		&c.CreatedAt, &lastSyncedAt, &c.SyncStatus, &c.SyncError, &videoCount, &c.SyncMode)
	if err != nil {
		return c, err
	}
	scanYTChannel(&c, groupID, lastSyncedAt, dateFrom, dateTo, videoCount)
	return c, nil
}

const ytChannelCols = `SELECT c.id,c.channel_id,c.handle,c.name,c.group_id,
	DATE_FORMAT(c.date_from,'%Y-%m-%d'),DATE_FORMAT(c.date_to,'%Y-%m-%d'),
	c.created_at,c.last_synced_at,c.sync_status,c.sync_error,
	(SELECT COUNT(*) FROM yt_videos v WHERE v.channel_id=c.channel_id) AS video_count,c.sync_mode
	FROM yt_channels c `

// GetYTChannel loads one channel by its row id.
func (s *Store) GetYTChannel(id string) (model.YTChannel, error) {
	return s.scanYTChannelRow(s.DB.QueryRow(ytChannelCols+"WHERE c.id=?", id))
}

// GetYTChannelByChannelID loads one channel by its UC... ID.
func (s *Store) GetYTChannelByChannelID(channelID string) (model.YTChannel, error) {
	return s.scanYTChannelRow(s.DB.QueryRow(ytChannelCols+"WHERE c.channel_id=?", channelID))
}

// YTChannelExists reports whether a channel row id exists.
func (s *Store) YTChannelExists(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM yt_channels WHERE id=?", id).Scan(&n)
	return n > 0
}

// CreateYTChannel inserts a new channel row (status "never" until the
// first sync runs). Callers check for duplicates first.
func (s *Store) CreateYTChannel(c model.YTChannel) error {
	mode := c.SyncMode
	if mode == "" {
		mode = "full"
	}
	_, err := s.DB.Exec(`INSERT INTO yt_channels(id,channel_id,handle,name,group_id,
		date_from,date_to,
		created_at,last_synced_at,sync_status,sync_error,sync_mode) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.ChannelID, c.Handle, c.Name, groupIDArg(c.GroupID),
		dateArg(c.DateFrom), dateArg(c.DateTo),
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

// UpdateYTChannel updates a channel's group and pull range. A nil
// dateFrom/dateTo clears that bound (NULL = no limit); groupID "" clears
// the group. Callers validate the dates; nothing is validated here.
func (s *Store) UpdateYTChannel(id string, groupID string, dateFrom, dateTo *string) error {
	_, err := s.DB.Exec(`UPDATE yt_channels SET group_id=?,date_from=?,date_to=? WHERE id=?`,
		groupIDArg(groupID), dateArg(dateFrom), dateArg(dateTo), id)
	return err
}

// DeleteYTChannel deletes the channel AND its videos (videos are keyed
// by channel_id value, no FK, so both deletes run in one transaction).
func (s *Store) DeleteYTChannel(id string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var channelID string
	if err := tx.QueryRow("SELECT channel_id FROM yt_channels WHERE id=?", id).Scan(&channelID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE s FROM yt_video_summaries s JOIN yt_videos v ON v.id=s.video_id
		WHERE v.channel_id=?`, channelID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM yt_videos WHERE channel_id=?", channelID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM yt_channels WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetYTChannelSyncStatus records a sync outcome (and its time) without
// touching the channel name or any videos.
func (s *Store) SetYTChannelSyncStatus(id, status, syncErr string) error {
	_, err := s.DB.Exec("UPDATE yt_channels SET last_synced_at=?,sync_status=?,sync_error=? WHERE id=?",
		Now(), status, syncErr, id)
	return err
}

// UpdateYTChannelMeta applies one sync's channel name + outcome.
func (s *Store) UpdateYTChannelMeta(id, name, status, syncErr string) error {
	_, err := s.DB.Exec("UPDATE yt_channels SET name=?,last_synced_at=?,sync_status=?,sync_error=? WHERE id=?",
		name, Now(), status, syncErr, id)
	return err
}

// UpsertYTVideos inserts new videos and updates re-fetched ones. The
// PRIMARY KEY on yt_videos.id (the video_id / playlist ID) makes a
// re-sync of the same item an update, never a duplicate row.
//
// A re-fetch must never clobber what is already known with an empty
// value: description and published_at are only overwritten when the
// incoming value is non-empty / non-NULL, and the same guard applies to
// duration and live_status (0 / "" mean "unknown" on the way in). Title,
// thumbnail, URL and kind are always overwritten — they always come
// from the flat listing. An empty PublishedAt becomes NULL (never a
// zero date); an empty Kind defaults to "video".
func (s *Store) UpsertYTVideos(channelID string, videos []model.YTVideo) error {
	if len(videos) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO yt_videos(id,channel_id,kind,title,description,
		published_at,duration,live_status,thumbnail_url,url,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE kind=VALUES(kind),title=VALUES(title),
		description=IF(VALUES(description)<>'',VALUES(description),description),
		published_at=IF(VALUES(published_at) IS NOT NULL,VALUES(published_at),published_at),
		duration=IF(VALUES(duration) IS NOT NULL,VALUES(duration),duration),
		live_status=IF(VALUES(live_status)<>'',VALUES(live_status),live_status),
		thumbnail_url=VALUES(thumbnail_url),url=VALUES(url)`)
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

// PruneYTVideosOutsideRange deletes a channel's video/short/live rows
// whose published_at falls outside [from, to] (date_to is inclusive of
// the whole day). A bound that is "" is not applied; with neither bound
// set this is a no-op. Playlists are never pruned — they have no date
// and the range does not apply to them. Rows with a NULL published_at
// are kept: they are candidates for the next sync's backfill, not stale
// data.
func (s *Store) PruneYTVideosOutsideRange(channelID, from, to string) error {
	if from == "" && to == "" {
		return nil
	}
	// Delete the complement of the range: before from, or on/after the
	// day after to. Only one side is tested when only one bound is set.
	var outside []string
	var args []any
	args = append(args, channelID)
	if from != "" {
		outside = append(outside, "published_at<?")
		args = append(args, from)
	}
	if to != "" {
		outside = append(outside, "published_at>=DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, to)
	}
	q := `DELETE FROM yt_videos WHERE channel_id=? AND kind IN ('video','short','live')
		AND added_manually=0 AND published_at IS NOT NULL AND (` + strings.Join(outside, " OR ") + ")"
	_, err := s.DB.Exec(q, args...)
	return err
}

// ListYTVideos returns videos newest first, with the channel name joined
// for display. Keyword matches title/description (LIKE); the filter
// always runs on the original text.
func (s *Store) ListYTVideos(f YTVideoFilter) ([]model.YTVideo, error) {
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
		conds = append(conds, "v.channel_id=?")
		args = append(args, f.ChannelID)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		conds = append(conds, "(v.title LIKE ? OR v.description LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	if f.Kind != "" {
		conds = append(conds, "v.kind=?")
		args = append(args, f.Kind)
	}
	if f.DateFrom != "" {
		conds = append(conds, "v.published_at>=?")
		args = append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		conds = append(conds, "v.published_at<DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.DateTo)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	if f.WatchLater {
		conds = append(conds, "v.watch_later=1")
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	q := `SELECT v.id,v.channel_id,COALESCE(c.name,''),v.kind,v.title,v.description,
		DATE_FORMAT(v.published_at,'%Y-%m-%d %H:%i:%s'),v.duration,v.live_status,v.thumbnail_url,v.url,v.created_at,
		COALESCE(DATE_FORMAT(v.watched_at,'%Y-%m-%d %H:%i:%s'),''),v.watch_later
		FROM yt_videos v LEFT JOIN yt_channels c ON c.channel_id=v.channel_id ` +
		where + ` ORDER BY v.published_at DESC, v.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.YTVideo{}
	for rows.Next() {
		var v model.YTVideo
		var publishedAt sql.NullString
		var duration sql.NullInt64
		if err := rows.Scan(&v.ID, &v.ChannelID, &v.ChannelName, &v.Kind, &v.Title,
			&v.Description, &publishedAt, &duration, &v.LiveStatus, &v.ThumbnailURL, &v.URL, &v.CreatedAt,
			&v.WatchedAt, &v.WatchLater); err != nil {
			return nil, err
		}
		// NULL published_at (playlists, or a video whose backfill has
		// not supplied a date yet) reads back as "". DATE_FORMAT keeps
		// the "2006-01-02 15:04:05" wall-time format whether or not
		// the DSN sets parseTime=true (a plain column scan would
		// yield RFC3339 under parseTime).
		if publishedAt.Valid {
			v.PublishedAt = publishedAt.String
		}
		if duration.Valid {
			v.Duration = int(duration.Int64)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- yt groups (same pattern as repo_groups) ----

func (s *Store) ListYTGroups() ([]model.Group, error) {
	rows, err := s.DB.Query("SELECT id,name,fixed FROM yt_groups ORDER BY id")
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

func (s *Store) CreateYTGroup(name string) (model.Group, error) {
	g := model.Group{ID: NewID("grp_"), Name: name}
	_, err := s.DB.Exec("INSERT INTO yt_groups(id,name,fixed) VALUES(?,?,0)", g.ID, g.Name)
	return g, err
}

// DeleteYTGroup moves the group's channels to the default group first,
// then deletes the group (mirrors DeleteRepoGroup).
func (s *Store) DeleteYTGroup(id string) (code string, err error) {
	var fixed bool
	err = s.DB.QueryRow("SELECT fixed FROM yt_groups WHERE id=?", id).Scan(&fixed)
	if err != nil {
		return "NOT_FOUND", err
	}
	if fixed {
		return "GROUP_FIXED", fmt.Errorf("fixed group cannot be deleted")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT IGNORE INTO yt_groups(id,name,fixed) VALUES('other','默认',1)"); err != nil {
		return "", err
	}
	if _, err := tx.Exec("UPDATE yt_channels SET group_id='other' WHERE group_id=?", id); err != nil {
		return "", err
	}
	if _, err := tx.Exec("DELETE FROM yt_groups WHERE id=?", id); err != nil {
		return "", err
	}
	return "", tx.Commit()
}

func (s *Store) YTGroupExists(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM yt_groups WHERE id=?", id).Scan(&n)
	return n > 0
}

// GetYTVideo returns one video by its YouTube ID (sql.ErrNoRows when
// unknown).
func (s *Store) GetYTVideo(id string) (model.YTVideo, error) {
	var v model.YTVideo
	var desc, published sql.NullString
	var duration sql.NullInt64
	err := s.DB.QueryRow(`SELECT id,channel_id,kind,title,description,
		DATE_FORMAT(published_at,'%Y-%m-%d %H:%i:%s'),duration,
		live_status,thumbnail_url,url FROM yt_videos WHERE id=?`, id).Scan(&v.ID, &v.ChannelID, &v.Kind,
		&v.Title, &desc, &published, &duration, &v.LiveStatus, &v.ThumbnailURL, &v.URL)
	if err != nil {
		return v, err
	}
	v.Description = desc.String
	v.PublishedAt = published.String
	v.Duration = int(duration.Int64)
	return v, nil
}

// GetYTSummary returns the summary row of (video, lang); ok=false when
// there is none.
func (s *Store) GetYTSummary(videoID, lang string) (sum model.YTSummary, ok bool, err error) {
	var summary, keyPoints, chapters, errText sql.NullString
	var resources string
	err = s.DB.QueryRow(`SELECT video_id,lang,status,summary,key_points,chapters,COALESCE(resources,''),source,transcript_lang,
		model,error,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s') FROM yt_video_summaries WHERE video_id=? AND lang=?`, videoID, lang).
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

// StartYTSummary marks (video, lang) pending, clearing any earlier
// result or error (a regenerate starts from scratch).
func (s *Store) StartYTSummary(videoID, lang string) error {
	now := Now()
	_, err := s.DB.Exec(`INSERT INTO yt_video_summaries(video_id,lang,status,summary,key_points,chapters,
		source,transcript_lang,model,error,created_at,updated_at) VALUES(?,?,'pending','','[]','[]','','','','',?,?)
		ON DUPLICATE KEY UPDATE status='pending',error='',updated_at=VALUES(updated_at)`, videoID, lang, now, now)
	return err
}

// FinishYTSummary stores a finished summary.
func (s *Store) FinishYTSummary(sum model.YTSummary) error {
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
	_, err = s.DB.Exec(`UPDATE yt_video_summaries SET status='done',summary=?,key_points=?,chapters=?,resources=?,
		source=?,transcript_lang=?,model=?,error='',updated_at=? WHERE video_id=? AND lang=?`,
		sum.Summary, string(kp), string(ch), string(rs), sum.Source, sum.TranscriptLang, sum.Model, Now(), sum.VideoID, sum.Lang)
	return err
}

// FailYTSummary records a failed run. An earlier finished summary (a
// failed regenerate) is kept; only the status and error change.
func (s *Store) FailYTSummary(videoID, lang, msg string) error {
	_, err := s.DB.Exec(`UPDATE yt_video_summaries SET status='failed',error=?,updated_at=?
		WHERE video_id=? AND lang=?`, msg, Now(), videoID, lang)
	return err
}

// SetYTChannelSyncMode switches a channel between full and single mode.
func (s *Store) SetYTChannelSyncMode(id, mode string) error {
	_, err := s.DB.Exec("UPDATE yt_channels SET sync_mode=? WHERE id=?", mode, id)
	return err
}

// MarkYTVideoManual flags a video as added by URL (kept by date pruning).
func (s *Store) MarkYTVideoManual(id string) error {
	_, err := s.DB.Exec("UPDATE yt_videos SET added_manually=1 WHERE id=?", id)
	return err
}

// ManualYTVideoURLs returns the URLs of a channel's videos added by URL.
func (s *Store) ManualYTVideoURLs(channelID string) ([]string, error) {
	rows, err := s.DB.Query("SELECT url FROM yt_videos WHERE channel_id=? AND added_manually=1", channelID)
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
