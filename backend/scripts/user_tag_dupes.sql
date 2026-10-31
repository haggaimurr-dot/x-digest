-- 用户标签疑似重复诊断（只读）。按 LOWER(tag) 分组，列出有多种写法的标签及各自使用次数。
-- 用法：mysql xdigest < backend/scripts/user_tag_dupes.sql
-- 全角字符的归一化只在 API（GET /api/v1/user-tags/duplicates）里做，这里只看大小写。
SELECT LOWER(tag) AS tag_key,
       GROUP_CONCAT(DISTINCT CONCAT(tag, '×', n) ORDER BY n DESC SEPARATOR ' | ') AS spellings,
       COUNT(*) AS variants
FROM (
  SELECT tag COLLATE utf8mb4_bin AS tag, COUNT(*) AS n
  FROM (SELECT tag FROM author_tags
        UNION ALL SELECT tag FROM user_tag_overrides
        UNION ALL SELECT tag FROM user_tag_vocabulary) x
  GROUP BY tag COLLATE utf8mb4_bin
) t
GROUP BY LOWER(tag)
HAVING COUNT(*) > 1
ORDER BY variants DESC, tag_key;

-- 合并前备份：
-- mysqldump xdigest author_tags user_tag_overrides user_tag_vocabulary > user_tags_backup.sql
