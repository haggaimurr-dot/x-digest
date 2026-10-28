package store

import (
	"database/sql"
	"errors"
	"testing"
)

// DB-backed (XDIGEST_TEST_DSN, see store_yt_test.go): rename conflicts,
// soft delete + restore keeps assignments, purge.
func TestCategoriesRenameDeleteRestore(t *testing.T) {
	st := testYTStore(t)
	if _, err := st.DB.Exec(`INSERT INTO posts(id,url,author,author_name,title,summary,published_at) VALUES('p1','u','a','A','t','s','2026-10-01 00:00:00')`); err != nil {
		t.Fatalf("seed post: %v", err)
	}
	a, err := st.CreateCategory("AI 工具")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := st.CreateCategory("教程")
	if _, err := st.CreateCategory("ai 工具"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("create duplicate (ci): %v", err)
	}
	if _, err := st.RenameCategory(b.ID, "AI 工具"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("rename to taken name: %v", err)
	}
	if _, err := st.RenameCategory(b.ID, "上手教程"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := st.RenameCategory("nope", "x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rename unknown: %v", err)
	}
	if err := st.SetPostCategories("p1", []string{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCategory(a.ID); err != nil {
		t.Fatal(err)
	}
	cats, asg, _ := st.ListCategories()
	for _, c := range cats {
		if c.ID == a.ID {
			t.Fatal("deleted category still listed")
		}
	}
	if len(asg["p1"]) != 1 {
		t.Fatalf("assignments after delete = %v", asg["p1"])
	}
	// Re-assigning live categories must not drop the deleted one's row.
	if err := st.SetPostCategories("p1", []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RestoreCategory(a.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	_, asg, _ = st.ListCategories()
	if len(asg["p1"]) != 2 {
		t.Fatalf("assignments after restore = %v", asg["p1"])
	}
	if _, err := st.RestoreCategory(a.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("restore live category: %v", err)
	}
	_ = st.DeleteCategory(b.ID)
	_, _ = st.DB.Exec("UPDATE categories SET deleted_at=NOW() - INTERVAL 8 DAY WHERE id=?", b.ID)
	if n, err := st.PurgeDeletedCategories(7); err != nil || n != 1 {
		t.Fatalf("purge: n=%d err=%v", n, err)
	}
}

func TestBatchCategory(t *testing.T) {
	st := testYTStore(t)
	for _, id := range []string{"p1", "p2", "p3", "p4", "p5"} {
		if _, err := st.DB.Exec(`INSERT INTO posts(id,published_at) VALUES(?, '2026-10-01 00:00:00')`, id); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := st.CreateCategory("A")
	if n, err := st.BatchCategory([]string{"p1", "p2", "p3", "p4", "p5", "nope"}, a.ID, true); err != nil || n != 5 {
		t.Fatalf("add: n=%d err=%v", n, err)
	}
	if n, _ := st.BatchCategory([]string{"p1", "p2"}, a.ID, true); n != 0 {
		t.Fatalf("re-add changed %d", n)
	}
	if n, err := st.BatchCategory([]string{"p1", "p2"}, a.ID, false); err != nil || n != 2 {
		t.Fatalf("remove: n=%d err=%v", n, err)
	}
	_, asg, _ := st.ListCategories()
	if len(asg) != 3 {
		t.Fatalf("posts in A = %d, want 3", len(asg))
	}
	if _, err := st.BatchCategory([]string{"p1"}, "missing", true); err == nil {
		t.Fatal("unknown category accepted")
	}
}

func TestCloneReplica(t *testing.T) {
	st := testYTStore(t)
	for _, q := range []string{
		"INSERT INTO replicas(id,post_id,post_title,created_at) VALUES('rep_a','p1','教程','2026-10-01 00:00:00')",
		"INSERT INTO replica_steps(replica_id,idx,text,status,note) VALUES('rep_a',0,'一','done','笔记'),('rep_a',1,'二','active','')",
		"INSERT INTO replica_steps(replica_id,idx,text,status,parent_idx) VALUES('rep_a',2,'二.1','done',1)",
	} {
		if _, err := st.DB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	r, err := st.CloneReplica("rep_a", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID == "rep_a" || r.PostTitle != "教程（副本）" || len(r.Steps) != 3 {
		t.Fatalf("clone = %+v", r)
	}
	for _, s := range r.Steps {
		if s.Status != "idle" || s.Note != "" || len(s.Attachments) != 0 {
			t.Fatalf("step not reset: %+v", s)
		}
	}
	if r.Steps[2].ParentIdx == nil || *r.Steps[2].ParentIdx != 1 {
		t.Fatal("sub-step parent lost")
	}
	if r2, _ := st.CloneReplica("rep_a", true); r2.Steps[0].Note != "笔记" {
		t.Fatal("copy_notes did not copy the note")
	}
}
