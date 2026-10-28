package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommentQueueDedupesAndBacksOff(t *testing.T) {
	q := newCommentQueue()
	if !q.enqueue("a") || !q.enqueue("a") {
		t.Fatal("a should be pending")
	}
	if len(q.ch) != 1 {
		t.Fatalf("a queued %d times, want once", len(q.ch))
	}
	<-q.ch
	q.done("a", true)
	if q.enqueue("a") {
		t.Fatal("a just failed: should not be re-queued yet")
	}
	q.done("b", false)
	if !q.enqueue("b") {
		t.Fatal("b should queue")
	}
}

func TestCommentsDisabledByDefault(t *testing.T) {
	called := false
	h := func(w http.ResponseWriter, r *http.Request) { called = true }
	for v, want := range map[string]int{"": 410, "false": 410, "true": 200, "1": 200} {
		called = false
		t.Setenv("COMMENTS_ENABLED", v)
		rec := httptest.NewRecorder()
		commentsHandler(h)(rec, httptest.NewRequest("GET", "/api/v1/yt/comments?ids=a", nil))
		if rec.Code != want || called != (want == 200) {
			t.Errorf("COMMENTS_ENABLED=%q: status %d called %v", v, rec.Code, called)
		}
	}
}
