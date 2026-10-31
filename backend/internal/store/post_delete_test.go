package store

import "testing"

// DB-backed (XDIGEST_TEST_DSN, see store_yt_test.go): delete cascades,
// leaves a tombstone, unknown ids are skipped, UpsertXPost restores.
func TestDeletePosts(t *testing.T) {
	st := testYTStore(t)
	for _, id := range []string{"p1", "p2", "p3"} {
		if _, err := st.DB.Exec(`INSERT INTO posts(id,url,author,author_name,title,summary,published_at) VALUES(?,'u','a','A','t','s','2026-10-01 00:00:00')`, id); err != nil {
			t.Fatalf("seed post: %v", err)
		}
		if _, err := st.DB.Exec(`INSERT INTO post_tags(post_id,tag) VALUES(?, 'AI')`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.DB.Exec(`INSERT INTO post_summaries(post_id,lang,status,created_at,updated_at) VALUES('p1','zh','done',NOW(),NOW())`); err != nil {
		t.Fatal(err)
	}
	n, err := st.DeletePosts([]string{"p1", "p2", "nope", "p1"})
	if err != nil || n != 2 {
		t.Fatalf("DeletePosts = %d, %v; want 2", n, err)
	}
	if st.PostExists("p1") || st.PostExists("p2") || !st.PostExists("p3") {
		t.Fatal("wrong posts deleted")
	}
	if !st.PostDeleted("p1") || !st.PostDeleted("p2") || st.PostDeleted("nope") || st.PostDeleted("p3") {
		t.Fatal("tombstones wrong")
	}
	var tags, sums int
	_ = st.DB.QueryRow("SELECT COUNT(*) FROM post_tags WHERE post_id IN ('p1','p2')").Scan(&tags)
	_ = st.DB.QueryRow("SELECT COUNT(*) FROM post_summaries WHERE post_id='p1'").Scan(&sums)
	if tags != 0 || sums != 0 {
		t.Fatalf("leftovers: tags=%d summaries=%d", tags, sums)
	}
	if n, _ := st.DeletePosts([]string{"p1"}); n != 0 {
		t.Fatalf("second delete = %d, want 0", n)
	}
	if err := st.UpsertXPost(XPostRow{ID: "p1", URL: "u", Author: "a", AuthorName: "A", PublishedAt: "2026-10-01T00:00:00Z", Title: "t"}); err != nil {
		t.Fatal(err)
	}
	if !st.PostExists("p1") || st.PostDeleted("p1") {
		t.Fatal("re-import should restore the post and clear the tombstone")
	}
}
