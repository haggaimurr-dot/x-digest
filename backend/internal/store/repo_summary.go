package store

// GitHub repo AI summaries (repo_summaries, 2026-10-29): same lifecycle as
// the post and video summaries (pending → done | failed, generated on
// demand), rows returned as model.YTSummary so the page reuses the summary
// UI. VideoID carries the repo full name; chapters are always empty.

import (
	"database/sql"
	"encoding/json"
	"errors"

	"xdigest/internal/model"
)

func (s *Store) GetRepoSummary(fullName, lang string) (sum model.YTSummary, ok bool, err error) {
	var summary, keyPoints, resources, errText sql.NullString
	err = s.DB.QueryRow(`SELECT full_name,lang,status,summary,key_points,resources,source,model,error,
		DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s')
		FROM repo_summaries WHERE full_name=? AND lang=?`, fullName, lang).
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

// StartRepoSummary marks (repo, lang) pending; an earlier result is kept
// until the new one replaces it.
func (s *Store) StartRepoSummary(fullName, lang string) error {
	now := Now()
	_, err := s.DB.Exec(`INSERT INTO repo_summaries(full_name,lang,status,summary,key_points,resources,source,model,error,
		created_at,updated_at) VALUES(?,?,'pending','','[]','[]','','','',?,?)
		ON DUPLICATE KEY UPDATE status='pending',error='',updated_at=VALUES(updated_at)`, fullName, lang, now, now)
	return err
}

func (s *Store) FinishRepoSummary(sum model.YTSummary) error {
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
	_, err = s.DB.Exec(`UPDATE repo_summaries SET status='done',summary=?,key_points=?,resources=?,source=?,model=?,
		error='',updated_at=? WHERE full_name=? AND lang=?`,
		sum.Summary, string(kp), string(rs), sum.Source, sum.Model, Now(), sum.VideoID, sum.Lang)
	return err
}

func (s *Store) FailRepoSummary(fullName, lang, msg string) error {
	_, err := s.DB.Exec(`UPDATE repo_summaries SET status='failed',error=?,updated_at=? WHERE full_name=? AND lang=?`,
		msg, Now(), fullName, lang)
	return err
}
