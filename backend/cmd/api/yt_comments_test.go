package main

import "testing"

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
