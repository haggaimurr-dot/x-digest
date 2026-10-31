package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"xdigest/internal/model"
)

// ---- GitHub repos ----

type RepoFilter struct {
	Group string
	Tag   string
	Sort  string // "stars" or "pushed_at" (default)
}

func nullInt(p *int) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(*p), Valid: true}
}

func nullStr(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func (s *Store) repoTags(fullName string) ([]string, error) {
	rows, err := s.DB.Query("SELECT tag FROM repo_tags WHERE full_name=? ORDER BY tag", fullName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := emptyStrSlice()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) scanRepoRow(r *model.Repo, description, readmeIntro, tutorialSteps, language, htmlURL, homepage sql.NullString,
	openIssues, openPRs sql.NullInt32, pushedAt, repoUpdatedAt, lastSyncedAt sql.NullString,
	groupID, note, syncError, createdAt, releasedAt sql.NullString) error {
	if createdAt.Valid {
		v := createdAt.String
		r.CreatedAt = &v
	}
	if releasedAt.Valid {
		v := releasedAt.String
		r.ReleasedAt = &v
	}
	r.Description = description.String
	r.ReadmeIntro = readmeIntro.String
	if tutorialSteps.Valid && strings.TrimSpace(tutorialSteps.String) != "" {
		var steps []model.TutorialStep
		if err := json.Unmarshal([]byte(tutorialSteps.String), &steps); err == nil {
			r.TutorialSteps = steps
		}
		// A corrupt tutorial_steps value degrades to nil rather than
		// failing the whole row query.
	}
	r.Language = language.String
	r.HTMLURL = htmlURL.String
	r.Homepage = homepage.String
	if openIssues.Valid {
		v := int(openIssues.Int32)
		r.OpenIssues = &v
	}
	if openPRs.Valid {
		v := int(openPRs.Int32)
		r.OpenPRs = &v
	}
	if pushedAt.Valid {
		v := pushedAt.String
		r.PushedAt = &v
	}
	if repoUpdatedAt.Valid {
		v := repoUpdatedAt.String
		r.RepoUpdatedAt = &v
	}
	if lastSyncedAt.Valid {
		v := lastSyncedAt.String
		r.LastSyncedAt = &v
	}
	r.GroupID = groupID.String
	r.Note = note.String
	r.SyncError = syncError.String
	return nil
}

const repoColumns = `full_name,owner,name,description,readme_intro,tutorial_steps,language,stars,forks,open_issues,open_prs,
	html_url,homepage,pushed_at,repo_updated_at,group_id,note,last_synced_at,sync_status,sync_error,
	DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),
	(SELECT DATE_FORMAT(MAX(rr.published_at),'%Y-%m-%d %H:%i:%s') FROM repo_releases rr
	 WHERE rr.full_name=repos.full_name AND rr.draft=0) AS released_at`

func (s *Store) scanRepos(rows *sql.Rows) ([]model.Repo, error) {
	out := []model.Repo{}
	for rows.Next() {
		var r model.Repo
		var description, readmeIntro, tutorialSteps, language, htmlURL, homepage sql.NullString
		var openIssues, openPRs sql.NullInt32
		var pushedAt, repoUpdatedAt, lastSyncedAt sql.NullString
		var groupID, note, syncError, createdAt, releasedAt sql.NullString
		if err := rows.Scan(&r.FullName, &r.Owner, &r.Name, &description, &readmeIntro, &tutorialSteps, &language,
			&r.Stars, &r.Forks, &openIssues, &openPRs, &htmlURL, &homepage,
			&pushedAt, &repoUpdatedAt, &groupID, &note, &lastSyncedAt,
			&r.SyncStatus, &syncError, &createdAt, &releasedAt); err != nil {
			return nil, err
		}
		if err := s.scanRepoRow(&r, description, readmeIntro, tutorialSteps, language, htmlURL, homepage, openIssues, openPRs,
			pushedAt, repoUpdatedAt, lastSyncedAt, groupID, note, syncError, createdAt, releasedAt); err != nil {
			return nil, err
		}
		tags, err := s.repoTags(r.FullName)
		if err != nil {
			return nil, err
		}
		r.Tags = tags
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListRepos(f RepoFilter) ([]model.Repo, error) {
	var conds []string
	var args []any
	if f.Group != "" {
		conds = append(conds, "repos.group_id=?")
		args = append(args, f.Group)
	}
	if f.Tag != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM repo_tags t WHERE t.full_name=repos.full_name AND t.tag=?)")
		args = append(args, f.Tag)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	order := "repos.pushed_at DESC, repos.full_name ASC"
	if f.Sort == "stars" {
		order = "repos.stars DESC, repos.full_name ASC"
	}
	rows, err := s.DB.Query("SELECT "+repoColumns+" FROM repos "+where+" ORDER BY "+order, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanRepos(rows)
}

func (s *Store) GetRepo(fullName string) (*model.Repo, error) {
	var r model.Repo
	var description, readmeIntro, tutorialSteps, language, htmlURL, homepage sql.NullString
	var openIssues, openPRs sql.NullInt32
	var pushedAt, repoUpdatedAt, lastSyncedAt sql.NullString
	var groupID, note, syncError, createdAt, releasedAt sql.NullString
	err := s.DB.QueryRow("SELECT "+repoColumns+" FROM repos WHERE full_name=?", fullName).Scan(
		&r.FullName, &r.Owner, &r.Name, &description, &readmeIntro, &tutorialSteps, &language,
		&r.Stars, &r.Forks, &openIssues, &openPRs, &htmlURL, &homepage,
		&pushedAt, &repoUpdatedAt, &groupID, &note, &lastSyncedAt,
		&r.SyncStatus, &syncError, &createdAt, &releasedAt)
	if err != nil {
		return nil, err
	}
	if err := s.scanRepoRow(&r, description, readmeIntro, tutorialSteps, language, htmlURL, homepage, openIssues, openPRs,
		pushedAt, repoUpdatedAt, lastSyncedAt, groupID, note, syncError, createdAt, releasedAt); err != nil {
		return nil, err
	}
	if r.Tags, err = s.repoTags(fullName); err != nil {
		return nil, err
	}
	if r.Commits, err = s.ListRepoCommits(fullName, 10); err != nil {
		return nil, err
	}
	rels, err := s.ListRepoReleases(fullName, 1)
	if err != nil {
		return nil, err
	}
	if len(rels) > 0 {
		r.LatestRelease = &rels[0]
	}
	return &r, nil
}

func (s *Store) RepoExists(fullName string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM repos WHERE full_name=?", fullName).Scan(&n)
	return n > 0
}

func (s *Store) CreateRepo(r model.Repo) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO repos(full_name,owner,name,description,language,stars,forks,
		open_issues,open_prs,html_url,homepage,pushed_at,repo_updated_at,group_id,note,
		last_synced_at,sync_status,sync_error,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.FullName, r.Owner, r.Name, r.Description, r.Language, r.Stars, r.Forks,
		nullInt(r.OpenIssues), nullInt(r.OpenPRs), r.HTMLURL, r.Homepage,
		nullStr(r.PushedAt), nullStr(r.RepoUpdatedAt), groupIDArg(r.GroupID), r.Note,
		nullStr(r.LastSyncedAt), "never", "", Now()); err != nil {
		return err
	}
	for _, t := range r.Tags {
		if _, err := tx.Exec("INSERT IGNORE INTO repo_tags(full_name,tag) VALUES(?,?)", r.FullName, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// groupIDArg maps "" to NULL so an empty group means "no group".
func groupIDArg(g string) any {
	if g == "" {
		return nil
	}
	return g
}

// PatchRepo updates only non-nil fields; tags (when non-nil) replace the set.
// A non-nil group_id of "" clears the group (NULL).
func (s *Store) PatchRepo(fullName string, groupID *string, tags *[]string, note *string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sets []string
	var args []any
	if groupID != nil {
		sets, args = append(sets, "group_id=?"), append(args, groupIDArg(*groupID))
	}
	if note != nil {
		sets, args = append(sets, "note=?"), append(args, *note)
	}
	if len(sets) > 0 {
		args = append(args, fullName)
		if _, err := tx.Exec("UPDATE repos SET "+strings.Join(sets, ",")+" WHERE full_name=?", args...); err != nil {
			return err
		}
	}
	if tags != nil {
		if _, err := tx.Exec("DELETE FROM repo_tags WHERE full_name=?", fullName); err != nil {
			return err
		}
		for _, t := range *tags {
			if _, err := tx.Exec("INSERT IGNORE INTO repo_tags(full_name,tag) VALUES(?,?)", fullName, t); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteRepo(fullName string) error {
	_, err := s.DB.Exec("DELETE FROM repos WHERE full_name=?", fullName)
	return err
}

// UpdateRepoMeta applies one sync's metadata to an existing row. Nil
// OpenIssues/OpenPRs write NULL (unknown, e.g. a failed search) rather
// than inventing a number. Commits/releases are upserted separately by
// the caller, only when that fetch succeeded (non-nil slices).
// ReadmeIntro is written as-is ("" when the README has no intro) and
// TutorialSteps as JSON, or NULL when there are none.
func (s *Store) UpdateRepoMeta(fullName string, r model.Repo, status, syncErr string) error {
	var tutorialSteps any
	if len(r.TutorialSteps) > 0 {
		b, err := json.Marshal(r.TutorialSteps)
		if err != nil {
			return err
		}
		tutorialSteps = string(b)
	}
	_, err := s.DB.Exec(`UPDATE repos SET description=?,readme_intro=?,tutorial_steps=?,language=?,stars=?,forks=?,
		open_issues=?,open_prs=?,html_url=?,homepage=?,pushed_at=?,repo_updated_at=?,
		last_synced_at=?,sync_status=?,sync_error=? WHERE full_name=?`,
		r.Description, r.ReadmeIntro, tutorialSteps, r.Language, r.Stars, r.Forks,
		nullInt(r.OpenIssues), nullInt(r.OpenPRs), r.HTMLURL, r.Homepage,
		nullStr(r.PushedAt), nullStr(r.RepoUpdatedAt), Now(), status, syncErr, fullName)
	return err
}

// SetRepoSyncStatus records a sync outcome without touching any synced
// data — used for rate_limited / failed so the old row is preserved.
func (s *Store) SetRepoSyncStatus(fullName, status, syncErr string) error {
	_, err := s.DB.Exec("UPDATE repos SET last_synced_at=?,sync_status=?,sync_error=? WHERE full_name=?",
		Now(), status, syncErr, fullName)
	return err
}

func (s *Store) UpsertRepoCommits(fullName string, commits []model.RepoCommit) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range commits {
		if _, err := tx.Exec(`INSERT INTO repo_commits(full_name,sha,message,author_name,author_login,committed_at,html_url)
			VALUES(?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE message=VALUES(message),author_name=VALUES(author_name),
			author_login=VALUES(author_login),committed_at=VALUES(committed_at),html_url=VALUES(html_url)`,
			fullName, c.SHA, c.Message, c.AuthorName, c.AuthorLogin, c.CommittedAt, c.HTMLURL); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertRepoReleases(fullName string, releases []model.RepoRelease) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, rel := range releases {
		if _, err := tx.Exec(`INSERT INTO repo_releases(full_name,tag_name,name,body,published_at,html_url,draft,prerelease)
			VALUES(?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE name=VALUES(name),body=VALUES(body),published_at=VALUES(published_at),
			html_url=VALUES(html_url),draft=VALUES(draft),prerelease=VALUES(prerelease)`,
			fullName, rel.TagName, rel.Name, rel.Body, nullStr(rel.PublishedAt), rel.HTMLURL, rel.Draft, rel.Prerelease); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListRepoCommits(fullName string, limit int) ([]model.RepoCommit, error) {
	rows, err := s.DB.Query(`SELECT sha,message,author_name,author_login,committed_at,html_url
		FROM repo_commits WHERE full_name=? ORDER BY committed_at DESC LIMIT ?`, fullName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RepoCommit{}
	for rows.Next() {
		var c model.RepoCommit
		if err := rows.Scan(&c.SHA, &c.Message, &c.AuthorName, &c.AuthorLogin, &c.CommittedAt, &c.HTMLURL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListRepoReleases(fullName string, limit int) ([]model.RepoRelease, error) {
	rows, err := s.DB.Query(`SELECT tag_name,name,body,published_at,html_url,draft,prerelease
		FROM repo_releases WHERE full_name=? ORDER BY published_at DESC LIMIT ?`, fullName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RepoRelease{}
	for rows.Next() {
		var rel model.RepoRelease
		var body, pub sql.NullString
		if err := rows.Scan(&rel.TagName, &rel.Name, &body, &pub, &rel.HTMLURL, &rel.Draft, &rel.Prerelease); err != nil {
			return nil, err
		}
		rel.Body = body.String
		if pub.Valid {
			v := pub.String
			rel.PublishedAt = &v
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// ---- repo groups ----

func (s *Store) ListRepoGroups() ([]model.Group, error) {
	rows, err := s.DB.Query("SELECT id,name,fixed FROM repo_groups ORDER BY id")
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

func (s *Store) CreateRepoGroup(name string) (model.Group, error) {
	g := model.Group{ID: NewID("grp_"), Name: name}
	_, err := s.DB.Exec("INSERT INTO repo_groups(id,name,fixed) VALUES(?,?,0)", g.ID, g.Name)
	return g, err
}

func (s *Store) DeleteRepoGroup(id string) (code string, err error) {
	var fixed bool
	err = s.DB.QueryRow("SELECT fixed FROM repo_groups WHERE id=?", id).Scan(&fixed)
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
	if _, err := tx.Exec("INSERT IGNORE INTO repo_groups(id,name,fixed) VALUES('other','默认',1)"); err != nil {
		return "", err
	}
	if _, err := tx.Exec("UPDATE repos SET group_id='other' WHERE group_id=?", id); err != nil {
		return "", err
	}
	if _, err := tx.Exec("DELETE FROM repo_groups WHERE id=?", id); err != nil {
		return "", err
	}
	return "", tx.Commit()
}

func (s *Store) RepoGroupExists(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM repo_groups WHERE id=?", id).Scan(&n)
	return n > 0
}
