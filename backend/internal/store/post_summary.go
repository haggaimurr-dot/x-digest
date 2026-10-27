package store

// X post AI summaries (post_summaries, 2026-10-27): same lifecycle as the
// video summaries (pending → done | failed, generated on demand), rows
// returned as model.YTSummary so the page reuses the video summary UI.
// VideoID carries the post id; chapters are always empty.

import (
	"database/sql"
	"encoding/json"
	"errors"

	"xdigest/internal/model"
)

func (s *Store) GetPostSummary(postID, lang string) (sum model.YTSummary, ok bool, err error) {
	var summary, keyPoints, resources, errText sql.NullString
	err = s.DB.QueryRow(`SELECT post_id,lang,status,summary,key_points,resources,source,model,error,
		DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s')
		FROM post_summaries WHERE post_id=? AND lang=?`, postID, lang).
		Scan(&sum.VideoID, &sum.Lang, &sum.Status, &summary, &keyPoints, &resources, &sum.Source,
			&sum.Model, &errText, &sum.CreatedAt, &sum.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sum, false, nil
	}
	if err != nil {
		return sum, false, err
	}
	sum.Summary, sum.Error = summary.String, errText.String
	sum.KeyPoints, sum.Chapters, sum.Resources = []string{}, []model.YTSummaryChapter{}, []model.YTSummaryResource{}
	if keyPoints.String != "" {
		_ = json.Unmarshal([]byte(keyPoints.String), &sum.KeyPoints)
	}
	if resources.String != "" {
		_ = json.Unmarshal([]byte(resources.String), &sum.Resources)
	}
	return sum, true, nil
}

// StartPostSummary marks (post, lang) pending; an earlier result is kept
// until the new one replaces it.
func (s *Store) StartPostSummary(postID, lang string) error {
	now := Now()
	_, err := s.DB.Exec(`INSERT INTO post_summaries(post_id,lang,status,summary,key_points,resources,source,model,error,
		created_at,updated_at) VALUES(?,?,'pending','','[]','[]','','','',?,?)
		ON DUPLICATE KEY UPDATE status='pending',error='',updated_at=VALUES(updated_at)`, postID, lang, now, now)
	return err
}

func (s *Store) FinishPostSummary(sum model.YTSummary) error {
	kp, err := json.Marshal(sum.KeyPoints)
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
	_, err = s.DB.Exec(`UPDATE post_summaries SET status='done',summary=?,key_points=?,resources=?,source=?,model=?,
		error='',updated_at=? WHERE post_id=? AND lang=?`,
		sum.Summary, string(kp), string(rs), sum.Source, sum.Model, Now(), sum.VideoID, sum.Lang)
	return err
}

func (s *Store) FailPostSummary(postID, lang, msg string) error {
	_, err := s.DB.Exec(`UPDATE post_summaries SET status='failed',error=?,updated_at=? WHERE post_id=? AND lang=?`,
		msg, Now(), postID, lang)
	return err
}
