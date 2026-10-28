package store

import (
	"strings"
)

// User-tag aliases and usage stats (2026-10-28).
//
// user_tag_aliases maps an alias (cc) to its canonical tag (claude-code).
// Writes go through canonicalTags with the alias map applied, and the page
// search "标签:cc" is resolved to the canonical tag before matching.

// TagAlias is one alias → canonical mapping.
type TagAlias struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
}

func (s *Store) ListTagAliases() ([]TagAlias, error) {
	rows, err := s.DB.Query("SELECT alias, canonical FROM user_tag_aliases ORDER BY alias")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TagAlias{}
	for rows.Next() {
		var a TagAlias
		if err := rows.Scan(&a.Alias, &a.Canonical); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetTagAlias upserts alias → canonical (both normalised).
func (s *Store) SetTagAlias(alias, canonical string) error {
	_, err := s.DB.Exec(`INSERT INTO user_tag_aliases(alias, canonical) VALUES(?,?)
		ON DUPLICATE KEY UPDATE canonical=VALUES(canonical)`, NormalizeTag(alias), NormalizeTag(canonical))
	return err
}

func (s *Store) DeleteTagAlias(alias string) (bool, error) {
	res, err := s.DB.Exec("DELETE FROM user_tag_aliases WHERE alias=?", NormalizeTag(alias))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// aliasMap: lower-cased alias → canonical. Errors read as "no aliases"
// (the table may not exist before schema.sql is re-applied).
func (s *Store) aliasMap() map[string]string {
	m := map[string]string{}
	as, err := s.ListTagAliases()
	if err != nil {
		return m
	}
	for _, a := range as {
		m[strings.ToLower(a.Alias)] = a.Canonical
	}
	return m
}

// applyAliases replaces aliases by their canonical tag (case-insensitive).
func applyAliases(tags []string, aliases map[string]string) []string {
	out := make([]string, len(tags))
	for i, t := range tags {
		if c, ok := aliases[strings.ToLower(NormalizeTag(t))]; ok {
			t = c
		}
		out[i] = t
	}
	return out
}

// TagStat is one user tag with how many authors carry it and how many of
// their posts fall in the requested date range.
type TagStat struct {
	Tag     string `json:"tag"`
	Authors int    `json:"authors"`
	Posts   int    `json:"posts"`
}

// UserTagStats counts effective user tags (an author's overrides replace
// their imported tags, as on the page). dateFrom/dateTo (YYYY-MM-DD,
// inclusive, may be empty) limit the post count.
func (s *Store) UserTagStats(dateFrom, dateTo string) ([]TagStat, error) {
	postCond, args := "", []any{}
	if dateFrom != "" {
		postCond += " AND p.published_at >= ?"
		args = append(args, dateFrom+" 00:00:00")
	}
	if dateTo != "" {
		postCond += " AND p.published_at < DATE_ADD(?, INTERVAL 1 DAY)"
		args = append(args, dateTo+" 00:00:00")
	}
	rows, err := s.DB.Query(`SELECT t.tag, COUNT(DISTINCT t.username), COUNT(p.id) FROM (
			SELECT username, tag FROM user_tag_overrides
			UNION
			SELECT at.username, at.tag FROM author_tags at
			WHERE NOT EXISTS (SELECT 1 FROM user_tag_overrides uo WHERE uo.username = at.username)
		) t LEFT JOIN posts p ON p.author = t.username`+postCond+`
		GROUP BY t.tag ORDER BY COUNT(p.id) DESC, COUNT(DISTINCT t.username) DESC, t.tag`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TagStat{}
	for rows.Next() {
		var st TagStat
		if err := rows.Scan(&st.Tag, &st.Authors, &st.Posts); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
