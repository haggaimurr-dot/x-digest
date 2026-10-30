package main

import (
	"testing"
	"time"
)

func TestNextDailyRun(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct{ now, want string }{
		{"2026-10-30 04:59", "2026-10-30 05:00"},
		{"2026-10-30 05:00", "2026-10-31 05:00"},
		{"2026-10-30 23:10", "2026-10-31 05:00"},
		{"2026-10-31 06:00", "2026-11-01 05:00"},
	} {
		if got := nextDailyRun(at(c.now).UTC(), 5, 0, loc); !got.Equal(at(c.want)) {
			t.Errorf("now %s → %v, want %s", c.now, got, c.want)
		}
	}
}

func TestDailySyncAt(t *testing.T) {
	for v, want := range map[string][3]int{"": {5, 0, 1}, "06:30": {6, 30, 1}, "off": {0, 0, 0}, "25:00": {0, 0, 0}} {
		t.Setenv("VIDEO_DAILY_SYNC_AT", v)
		h, m, ok := dailySyncAt()
		if h != want[0] || m != want[1] || ok != (want[2] == 1) {
			t.Errorf("%q → %d %d %v", v, h, m, ok)
		}
	}
}
