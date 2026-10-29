package store

import "strings"

// buildPostWhere turns a PostFilter into "WHERE …" plus its args. Pure, so
// the filter semantics are unit-tested without a database
// (post_filter_test.go). Empty filter → "".
func buildPostWhere(f PostFilter) (string, []any) {
	var conds []string
	var args []any
	in := func(col string, vals []string) {
		conds = append(conds, col+" IN ("+strings.TrimSuffix(strings.Repeat("?,", len(vals)), ",")+")")
		for _, v := range vals {
			args = append(args, v)
		}
	}
	if len(f.Authors) > 0 {
		in("p.author", f.Authors)
	}
	tags := clean(append([]string{f.Tag}, f.Tags...), "#")
	if len(tags) > 0 {
		// BINARY collation: the legacy JS match is case-sensitive, and the
		// source has both "workbuddy" and "Workbuddy" as distinct tags.
		ph := strings.TrimSuffix(strings.Repeat("?,", len(tags)), ",")
		conds = append(conds, "EXISTS (SELECT 1 FROM post_tags t WHERE t.post_id=p.id AND t.tag COLLATE utf8mb4_bin IN ("+ph+"))")
		for _, t := range tags {
			args = append(args, t)
		}
	}
	cats := clean(append([]string{f.Category}, f.Categories...), "")
	if len(cats) > 0 {
		var ors []string
		var ids []string
		for _, c := range cats {
			if c == "__uncat" {
				ors = append(ors, "NOT EXISTS (SELECT 1 FROM post_categories pc JOIN categories c ON c.id=pc.category_id WHERE pc.post_id=p.id AND c.deleted_at IS NULL)")
			} else {
				ids = append(ids, c)
			}
		}
		if len(ids) > 0 {
			ors = append(ors, "EXISTS (SELECT 1 FROM post_categories pc WHERE pc.post_id=p.id AND pc.category_id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+"))")
			for _, id := range ids {
				args = append(args, id)
			}
		}
		conds = append(conds, "("+strings.Join(ors, " OR ")+")")
	}
	if f.Kind == "未分类" {
		conds = append(conds, "COALESCE(p.kind,'')=''")
	} else if f.Kind != "" {
		conds = append(conds, "p.kind=?")
		args = append(args, f.Kind)
	}
	if f.MinChars > 0 {
		// Same fallback as the frontend postChars: text, then summary, then title.
		conds = append(conds, "CHAR_LENGTH(TRIM(COALESCE(NULLIF(p.text, ''), p.summary, p.title))) >= ?")
		args = append(args, f.MinChars)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + strings.TrimLeft(q, "#") + "%"
		conds = append(conds, `(p.title LIKE ? OR p.summary LIKE ? OR p.industry LIKE ? OR p.tech LIKE ?
			OR p.author_name LIKE ? OR p.author LIKE ? OR p.text LIKE ?
			OR EXISTS (SELECT 1 FROM post_tags t WHERE t.post_id=p.id AND t.tag LIKE ?))`)
		for i := 0; i < 8; i++ {
			args = append(args, like)
		}
	}
	if f.DateFrom != "" {
		conds = append(conds, "p.published_at >= ?")
		args = append(args, f.DateFrom+" 00:00:00")
	}
	if f.DateTo != "" {
		conds = append(conds, "p.published_at < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.DateTo+" 00:00:00")
	}
	if len(conds) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

func postOrder(sort string) string {
	if sort == "oldest" {
		return "p.published_at ASC, p.id ASC"
	}
	return "p.published_at DESC, p.id ASC"
}

// clean drops empty values and duplicates, trimming the given prefix.
func clean(vals []string, trimPrefix string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if trimPrefix != "" {
			v = strings.TrimPrefix(v, trimPrefix)
		}
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
