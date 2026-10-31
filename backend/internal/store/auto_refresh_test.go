package store

import (
	"reflect"
	"testing"
)

// DB-backed (XDIGEST_TEST_DSN, see store_yt_test.go): the set is replaced
// as a whole, unknown names are ignored, an empty list clears it.
func TestAutoRefreshSet(t *testing.T) {
	st := testYTStore(t)
	for _, u := range []string{"a", "b", "c"} {
		if _, err := st.DB.Exec("INSERT INTO authors(username,name) VALUES(?,?)", u, u); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.AutoRefreshUsernames()
	if err != nil || len(got) != 0 {
		t.Fatalf("initial = %v, %v; want empty", got, err)
	}
	if err := st.SetAutoRefresh([]string{"c", "a", "nope"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.AutoRefreshUsernames(); !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Fatalf("after set = %v; want [a c]", got)
	}
	if err := st.SetAutoRefresh([]string{"b"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.AutoRefreshUsernames(); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("after replace = %v; want [b]", got)
	}
	if err := st.SetAutoRefresh(nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.AutoRefreshUsernames(); len(got) != 0 {
		t.Fatalf("after clear = %v; want empty", got)
	}
}
