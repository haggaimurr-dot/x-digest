package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRepoSyncerJob(t *testing.T) {
	release := make(chan struct{})
	rs := &repoSyncer{
		list: func() ([]string, error) { return []string{"a/ok", "a/partial", "a/limited", "a/broken"}, nil },
		sync: func(ctx context.Context, name string) (string, error) {
			<-release
			switch name {
			case "a/ok":
				return "synced", nil
			case "a/partial":
				return "partial", nil
			case "a/limited":
				return "rate_limited", nil
			}
			return "", errors.New("boom")
		},
	}
	if ok, _ := rs.start(); !ok {
		t.Fatal("first start refused")
	}
	if ok, _ := rs.start(); ok {
		t.Fatal("second start while running accepted (want 409)")
	}
	rs.waiting(time.Date(2026, 10, 28, 9, 30, 0, 0, time.UTC))
	if st := rs.status(); !st.Running || st.Total != 4 || st.WaitingUntil != "2026-10-28T09:30:00Z" {
		t.Fatalf("running status %+v", st)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for rs.status().Running {
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	st := rs.status()
	if st.Done != 4 || st.Synced != 2 || st.RateLimited != 1 || st.Failed != 1 || st.FinishedAt == "" || st.WaitingUntil != "" {
		t.Fatalf("final status %+v", st)
	}
}
