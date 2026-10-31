package store

import (
	"strings"

	"xdigest/internal/translate"
)

// ---- translation cache ----
// Store implements translate.Cache. The key is translate.Hash(source,
// target); only genuine translations are ever stored (short-circuited
// originals and failed items are never cached).

func (s *Store) GetTranslations(hashes []string, target string) (map[string]string, error) {
	out := map[string]string{}
	if len(hashes) == 0 {
		return out, nil
	}
	ph := strings.Repeat("?,", len(hashes))
	q := "SELECT source_hash, translated_text FROM translations WHERE target_lang=? AND source_hash IN (" + strings.TrimSuffix(ph, ",") + ")"
	args := make([]any, 0, len(hashes)+1)
	args = append(args, target)
	for _, h := range hashes {
		args = append(args, h)
	}
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h, tr string
		if err := rows.Scan(&h, &tr); err != nil {
			return nil, err
		}
		out[h] = tr
	}
	return out, rows.Err()
}

// PurgeTranslations deletes cached translations older than days.
func (s *Store) PurgeTranslations(days int) (int64, error) {
	res, err := s.DB.Exec("DELETE FROM translations WHERE created_at < NOW() - INTERVAL ? DAY", days)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) PutTranslations(target string, items map[string]string) error {
	if len(items) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("INSERT INTO translations (source_hash, target_lang, source_text, translated_text) VALUES ")
	args := make([]any, 0, len(items)*4)
	first := true
	for src, tr := range items {
		if !first {
			sb.WriteString(",")
		}
		first = false
		sb.WriteString("(?,?,?,?)")
		args = append(args, translate.Hash(src, target), target, src, tr)
	}
	sb.WriteString(" ON DUPLICATE KEY UPDATE source_text=VALUES(source_text), translated_text=VALUES(translated_text)")
	_, err := s.DB.Exec(sb.String(), args...)
	return err
}
