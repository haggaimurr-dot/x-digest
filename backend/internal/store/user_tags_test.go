package store

import (
	"reflect"
	"testing"
)

func TestNormalizeTag(t *testing.T) {
	for in, want := range map[string]string{
		"  Claude　Code ": "Claude Code",
		"ＡＩ工具":           "AI工具",
		"ｗｏｒｋｂｕｄｄｙ":      "workbuddy",
		"a\t\tb":         "a b",
		"":               "",
	} {
		if got := NormalizeTag(in); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalTags(t *testing.T) {
	got := canonicalTags([]string{"WORKBUDDY", " ＡＩ ", "ai", "新标签", ""}, []string{"workbuddy", "AI"})
	want := []string{"workbuddy", "AI", "新标签"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonicalTags = %v, want %v", got, want)
	}
}

func TestGroupDuplicates(t *testing.T) {
	got := groupDuplicates([]TagCount{{"Workbuddy", 2}, {"AI", 5}, {"workbuddy", 7}, {"WORKBUDDY", 1}, {"ａｉ", 1}, {"solo", 3}})
	want := [][]TagCount{
		{{"workbuddy", 7}, {"Workbuddy", 2}, {"WORKBUDDY", 1}},
		{{"AI", 5}, {"ａｉ", 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groupDuplicates = %v\nwant %v", got, want)
	}
}

// DB-backed: merge Workbuddy/WB → workbuddy across the tables; a new
// WORKBUDDY then lands on the existing spelling.
func TestMergeUserTags(t *testing.T) {
	st := testYTStore(t)
	for _, q := range []string{
		"INSERT INTO authors(username,name) VALUES('u1','U1'),('u2','U2'),('u3','U3')",
		"INSERT INTO author_tags(username,tag) VALUES('u1','Workbuddy'),('u2','workbuddy'),('u3','WB')",
		"INSERT INTO user_tag_overrides(username,tag) VALUES('u3','Workbuddy')",
		"INSERT INTO user_tag_vocabulary(tag) VALUES('Workbuddy')",
	} {
		if _, err := st.DB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := st.MergeUserTags([]string{"Workbuddy", "WB"}, "workbuddy"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = st.DB.QueryRow("SELECT COUNT(*) FROM author_tags WHERE tag COLLATE utf8mb4_bin='workbuddy'").Scan(&n)
	if n != 3 {
		t.Fatalf("author_tags workbuddy = %d, want 3", n)
	}
	_ = st.DB.QueryRow("SELECT COUNT(*) FROM author_tags WHERE tag COLLATE utf8mb4_bin<>'workbuddy'").Scan(&n)
	if n != 0 {
		t.Fatalf("leftover variants = %d", n)
	}
	if err := st.SetUserTagOverride("u1", []string{"WORKBUDDY"}); err != nil {
		t.Fatal(err)
	}
	_, ov, _ := st.UserTags()
	if !reflect.DeepEqual(ov["u1"], []string{"workbuddy"}) {
		t.Fatalf("override = %v", ov["u1"])
	}
}

func TestApplyAliases(t *testing.T) {
	got := applyAliases([]string{"CC", "ｃｃ", "agent"}, map[string]string{"cc": "claude-code"})
	if !reflect.DeepEqual(got, []string{"claude-code", "claude-code", "agent"}) {
		t.Fatalf("applyAliases = %v", got)
	}
	if got := canonicalTags(applyAliases([]string{"cc", "claude-code"}, map[string]string{"cc": "claude-code"}), nil); !reflect.DeepEqual(got, []string{"claude-code"}) {
		t.Fatalf("alias + canonical dedupe = %v", got)
	}
}

// DB-backed: alias search and stats.
func TestTagAliasesAndStats(t *testing.T) {
	st := testYTStore(t)
	for _, q := range []string{
		"INSERT INTO authors(username,name) VALUES('u1','U1'),('u2','U2')",
		"INSERT INTO author_tags(username,tag) VALUES('u1','claude-code'),('u2','claude-code'),('u2','agent')",
		"INSERT INTO user_tag_overrides(username,tag) VALUES('u2','claude-code')",
		"INSERT INTO posts(id,author,published_at) VALUES('p1','u1','2026-10-01 10:00:00'),('p2','u1','2026-09-01 10:00:00'),('p3','u2','2026-10-02 10:00:00')",
	} {
		if _, err := st.DB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := st.SetTagAlias("cc", "claude-code"); err != nil {
		t.Fatal(err)
	}
	_, total, _, err := st.ListPosts(PostFilter{Query: "标签:cc", Page: 1, PageSize: 10})
	if err != nil || total != 3 {
		t.Fatalf("标签:cc total=%d err=%v, want 3", total, err)
	}
	stats, err := st.UserTagStats("2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	// u2's override replaces its imported "agent": only claude-code counts.
	if len(stats) != 1 || stats[0].Tag != "claude-code" || stats[0].Authors != 2 || stats[0].Posts != 2 {
		t.Fatalf("stats = %+v", stats)
	}
}
