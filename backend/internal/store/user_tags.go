package store

import (
	"strings"
	"unicode"
)

// User tags (author_tags imported from the watchlist, user_tag_overrides
// edited on the page, user_tag_vocabulary = the 常用标签 list).
//
// 2026-10-28: tags are normalised on write (NormalizeTag) and spelled like
// an existing tag that differs only in case (canonicalTags), so "WORKBUDDY"
// lands on the stored "workbuddy" instead of adding a variant. Existing
// variants are listed by UserTagDuplicates and folded by MergeUserTags.

// NormalizeTag trims, folds full-width ASCII (Ａ-Ｚ, ０-９, ＠ …) and the
// ideographic space to half-width, and collapses inner whitespace.
func NormalizeTag(t string) string {
	var b strings.Builder
	for _, r := range t {
		switch {
		case r == 0x3000:
			r = ' '
		case r >= 0xFF01 && r <= 0xFF5E:
			r -= 0xFEE0
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.FieldsFunc(b.String(), unicode.IsSpace), " ")
}

// canonicalTags normalises tags, drops empties and case-insensitive
// duplicates, and replaces each with the spelling already in known (first
// one wins) when only the case differs.
func canonicalTags(tags, known []string) []string {
	spell := map[string]string{}
	for _, k := range known {
		if k = NormalizeTag(k); k != "" {
			if _, ok := spell[strings.ToLower(k)]; !ok {
				spell[strings.ToLower(k)] = k
			}
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, t := range tags {
		t = NormalizeTag(t)
		key := strings.ToLower(t)
		if t == "" || seen[key] {
			continue
		}
		seen[key] = true
		if k, ok := spell[key]; ok {
			t = k
		}
		out = append(out, t)
	}
	return out
}

// knownUserTags: every spelling in use, vocabulary first (its spelling wins).
func (s *Store) knownUserTags() ([]string, error) {
	rows, err := s.DB.Query(`SELECT tag FROM user_tag_vocabulary
		UNION ALL SELECT tag FROM author_tags UNION ALL SELECT tag FROM user_tag_overrides`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) UserTags() (vocab []string, overrides map[string][]string, err error) {
	rows, err := s.DB.Query("SELECT tag FROM user_tag_vocabulary ORDER BY tag")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	vocab = emptyStrSlice()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, nil, err
		}
		vocab = append(vocab, t)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	orows, err := s.DB.Query("SELECT username,tag FROM user_tag_overrides ORDER BY username,tag")
	if err != nil {
		return nil, nil, err
	}
	defer orows.Close()
	overrides = map[string][]string{}
	for orows.Next() {
		var u, t string
		if err := orows.Scan(&u, &t); err != nil {
			return nil, nil, err
		}
		overrides[u] = append(overrides[u], t)
	}
	return vocab, overrides, orows.Err()
}

// SetUserTagVocabulary replaces the user-tag vocabulary (normalised, no
// case-insensitive duplicates, existing spellings kept). Authors' own tags
// are untouched.
func (s *Store) SetUserTagVocabulary(tags []string) error {
	known, err := s.knownUserTags()
	if err != nil {
		return err
	}
	tags = canonicalTags(applyAliases(tags, s.aliasMap()), append(append([]string{}, tags...), known...))
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM user_tag_vocabulary"); err != nil {
		return err
	}
	for _, t := range tags {
		if _, err := tx.Exec("INSERT INTO user_tag_vocabulary(tag) VALUES(?)", t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetUserTagOverride(username string, tags []string) error {
	known, err := s.knownUserTags()
	if err != nil {
		return err
	}
	tags = canonicalTags(applyAliases(tags, s.aliasMap()), known)
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM user_tag_overrides WHERE username=?", username); err != nil {
		return err
	}
	for _, t := range tags {
		if _, err := tx.Exec("INSERT INTO user_tag_overrides(username,tag) VALUES(?,?)", username, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TagCount is one tag spelling and how often it is used.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// UserTagDuplicates lists groups of user-tag spellings that are equal after
// NormalizeTag + lower-casing (the 疑似重复 list), with use counts.
// backend/scripts/user_tag_dupes.sql is the same check in plain SQL.
func (s *Store) UserTagDuplicates() ([][]TagCount, error) {
	rows, err := s.DB.Query(`SELECT tag COLLATE utf8mb4_bin, COUNT(*) FROM (
		SELECT tag FROM author_tags UNION ALL SELECT tag FROM user_tag_overrides UNION ALL SELECT tag FROM user_tag_vocabulary
	) x GROUP BY tag COLLATE utf8mb4_bin`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []TagCount
	for rows.Next() {
		var c TagCount
		if err := rows.Scan(&c.Tag, &c.Count); err != nil {
			return nil, err
		}
		all = append(all, c)
	}
	return groupDuplicates(all), rows.Err()
}

// groupDuplicates keeps only groups with 2+ spellings, most-used first.
func groupDuplicates(all []TagCount) [][]TagCount {
	byKey := map[string][]TagCount{}
	var order []string
	for _, c := range all {
		k := strings.ToLower(NormalizeTag(c.Tag))
		if _, ok := byKey[k]; !ok {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], c)
	}
	out := [][]TagCount{}
	for _, k := range order {
		g := byKey[k]
		if len(g) < 2 {
			continue
		}
		for i := 1; i < len(g); i++ { // tiny groups: insertion sort by count desc
			for j := i; j > 0 && g[j].Count > g[j-1].Count; j-- {
				g[j], g[j-1] = g[j-1], g[j]
			}
		}
		out = append(out, g)
	}
	return out
}

// MergeUserTags renames every from-tag to `to` (exact, case-sensitive match)
// in author_tags, user_tag_overrides and the vocabulary, in one transaction,
// and returns the number of rows changed. An author that already has `to`
// just loses the from-tag (no duplicate).
func (s *Store) MergeUserTags(from []string, to string) (int64, error) {
	to = NormalizeTag(to)
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var changed int64
	for _, f := range from {
		if f == to || f == "" {
			continue
		}
		for _, tbl := range []string{"author_tags", "user_tag_overrides"} {
			// The PK (username, tag) is case-insensitive, so a user has at
			// most one case variant: UPDATE IGNORE renames it unless the user
			// already has `to` in another row, then the DELETE drops it.
			for _, q := range []string{
				"UPDATE IGNORE " + tbl + " SET tag=? WHERE tag COLLATE utf8mb4_bin = ?",
				"DELETE FROM " + tbl + " WHERE tag COLLATE utf8mb4_bin = ?",
			} {
				args := []any{f}
				if strings.HasPrefix(q, "UPDATE") {
					args = []any{to, f}
				}
				res, err := tx.Exec(q, args...)
				if err != nil {
					return 0, err
				}
				n, _ := res.RowsAffected()
				changed += n
			}
		}
		if _, err := tx.Exec("DELETE FROM user_tag_vocabulary WHERE tag COLLATE utf8mb4_bin = ?", f); err != nil {
			return 0, err
		}
	}
	// Vocabulary PK is case-insensitive too: replace whatever spelling is
	// there with `to`.
	if _, err := tx.Exec("DELETE FROM user_tag_vocabulary WHERE tag = ?", to); err != nil {
		return 0, err
	}
	if _, err := tx.Exec("INSERT INTO user_tag_vocabulary(tag) VALUES(?)", to); err != nil {
		return 0, err
	}
	return changed, tx.Commit()
}
