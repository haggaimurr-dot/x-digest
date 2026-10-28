package main

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// memJobs is an in-memory refreshJobStore.
type memJobs struct {
	mu    sync.Mutex
	state []byte
	saves int
}

func (m *memJobs) SaveRefreshJob(id string, running bool, state []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state, m.saves = append([]byte{}, state...), m.saves+1
	return nil
}

func (m *memJobs) LatestRefreshJob() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == nil {
		return nil, sql.ErrNoRows
	}
	return m.state, nil
}

// A job cut off by a restart after 2 of 5 accounts resumes with only the
// other 3 (the done ones never reach Grok again) and ends with done=5.
func TestRefreshResumeAfterRestart(t *testing.T) {
	saved := refreshJob{ID: "rj_1", Running: true, StartedAt: "2026-10-28T08:00:00Z", Total: 5, Done: 2, Added: 3,
		Accounts: []*refreshAccount{
			{Username: "a", State: "done", Added: 1},
			{Username: "b", State: "done", Added: 2},
			{Username: "c", State: "running"},
			{Username: "d", State: "pending"},
			{Username: "e", State: "pending"},
		}}
	b, _ := json.Marshal(saved)
	jobs := &memJobs{state: b}

	var mu sync.Mutex
	var processed []string
	x := &xImporter{jobs: jobs}
	x.doAccount = func(a *refreshAccount) {
		mu.Lock()
		processed = append(processed, a.Username)
		mu.Unlock()
		x.set(func() { a.State, a.Added = "done", 1 })
	}
	x.resume()

	deadline := time.Now().Add(3 * time.Second)
	for {
		x.mu.Lock()
		running := x.job != nil && x.job.Running
		x.mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("resumed job did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(processed) != 3 {
		t.Fatalf("processed %v, want only c,d,e", processed)
	}
	for _, u := range processed {
		if u == "a" || u == "b" {
			t.Fatalf("done account %s processed again", u)
		}
	}
	var final refreshJob
	_ = json.Unmarshal(jobs.state, &final)
	if final.Running || final.Done != 5 || final.Added != 6 || final.FinishedAt == "" {
		t.Fatalf("final saved job: running=%v done=%d added=%d finished=%q", final.Running, final.Done, final.Added, final.FinishedAt)
	}
	if jobs.saves < 4 { // after each of 3 accounts + finish
		t.Fatalf("saves = %d", jobs.saves)
	}
}

// A finished job is shown after a restart but not re-run.
func TestRefreshResumeFinishedJob(t *testing.T) {
	b, _ := json.Marshal(refreshJob{ID: "rj_2", Running: false, Total: 1, Done: 1, Accounts: []*refreshAccount{{Username: "a", State: "done"}}})
	x := &xImporter{jobs: &memJobs{state: b}}
	x.doAccount = func(*refreshAccount) { t.Fatal("finished job re-run") }
	x.resume()
	if x.job == nil || x.job.ID != "rj_2" {
		t.Fatal("last job not loaded")
	}
	// No saved job at all: nothing happens.
	x2 := &xImporter{jobs: &memJobs{}}
	x2.resume()
	if x2.job != nil {
		t.Fatal("job from nowhere")
	}
}

// Retry: only accounts with failed posts are re-run, carrying those ids and
// the original since; no failures → 422; a running job → 409.
func TestRefreshRetryFailed(t *testing.T) {
	x := &xImporter{jobs: &memJobs{}}
	x.job = &refreshJob{ID: "rj_1", Accounts: []*refreshAccount{
		{Username: "a", State: "done", Since: "2026-10-01T00:00:00Z", Failed: 2, FailedIDs: []string{"11", "12"}},
		{Username: "b", State: "done"},
	}}
	accs := x.failedOfLastJob()
	if len(accs) != 1 || accs[0].Username != "a" || len(accs[0].RetryIDs) != 2 || accs[0].Since != "2026-10-01T00:00:00Z" {
		t.Fatalf("failedOfLastJob = %+v", accs)
	}
	got := make(chan *refreshAccount, 2)
	hold := make(chan struct{})
	x.doAccount = func(a *refreshAccount) { got <- a; <-hold; x.set(func() { a.State = "done" }) }
	if !x.startJob(accs) {
		t.Fatal("startJob refused")
	}
	a := <-got
	if a.Username != "a" || len(a.RetryIDs) != 2 {
		t.Fatalf("retry account %+v", a)
	}
	// While the retry job runs: 409.
	rec := httptest.NewRecorder()
	x.handleRetry(rec, httptest.NewRequest("POST", "/api/v1/x/refresh/retry", nil))
	close(hold)
	if rec.Code != 409 {
		t.Fatalf("retry while running: %d", rec.Code)
	}
	// Nothing failed: 422.
	x2 := &xImporter{job: &refreshJob{Accounts: []*refreshAccount{{Username: "b", State: "done"}}}}
	rec = httptest.NewRecorder()
	x2.handleRetry(rec, httptest.NewRequest("POST", "/api/v1/x/refresh/retry", nil))
	if rec.Code != 422 {
		t.Fatalf("retry with nothing failed: %d", rec.Code)
	}
}
