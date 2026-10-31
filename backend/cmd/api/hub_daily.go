package main

import (
	"log"
	"os"
	"time"
)

// 特别关注 (2026-10-30): channels / uploaders in the fixed "starred" group
// of each video hub are synced every day at VIDEO_DAILY_SYNC_AT
// (Asia/Shanghai wall time, "HH:MM", default 05:00; "off" disables). Other
// groups are only synced by hand (同步 / 同步全部).

const starredGroupID = "starred"

// dailySyncAt parses VIDEO_DAILY_SYNC_AT into hour and minute; ok=false
// when disabled or malformed.
func dailySyncAt() (hour, minute int, ok bool) {
	v := os.Getenv("VIDEO_DAILY_SYNC_AT")
	if v == "" {
		v = "05:00"
	}
	t, err := time.Parse("15:04", v)
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

// nextDailyRun is the first hour:minute in loc strictly after now.
func nextDailyRun(now time.Time, hour, minute int, loc *time.Location) time.Time {
	n := now.In(loc)
	next := time.Date(n.Year(), n.Month(), n.Day(), hour, minute, 0, 0, loc)
	if !next.After(n) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// scheduleStarredSync runs syncStarred on every hub once a day.
func scheduleStarredSync(hubs ...*videoHub) {
	hour, minute, ok := dailySyncAt()
	if !ok {
		log.Printf("daily starred video sync: off")
		return
	}
	for _, h := range hubs {
		if err := h.st.EnsureGroup(starredGroupID, "特别关注"); err != nil {
			log.Printf("%s: ensure starred group: %v", h.prefix, err)
		}
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	go func() {
		for {
			time.Sleep(time.Until(nextDailyRun(time.Now(), hour, minute, loc)))
			for _, h := range hubs {
				h.syncStarred()
			}
		}
	}()
}

// syncStarred syncs every channel of the starred group, one at a time.
func (h *videoHub) syncStarred() {
	channels, err := h.st.ListChannels()
	if err != nil {
		log.Printf("%s daily sync: %v", h.prefix, err)
		return
	}
	n, failed := 0, 0
	for _, ch := range channels {
		if ch.GroupID != starredGroupID {
			continue
		}
		n++
		updated, err := h.syncChannel(ch.ID)
		if err != nil || updated.SyncStatus != "synced" {
			failed++
			log.Printf("%s daily sync %s: %v", h.prefix, ch.ChannelID, err)
		}
	}
	log.Printf("%s daily sync: %d starred channels, %d failed", h.prefix, n, failed)
}
