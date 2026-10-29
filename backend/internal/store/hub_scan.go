package store

import (
	"database/sql"

	"xdigest/internal/model"
)

// YTVideoFilter filters HubStore.ListVideos. DateFrom/DateTo are
// "2006-01-02" calendar dates (Shanghai day bounds, like the post date
// filter). Kind is one of the platform's kinds; "" means all kinds.
// Limit <= 0 means the default 100.
type YTVideoFilter struct {
	ChannelID string
	Q         string
	DateFrom  string
	DateTo    string
	Kind      string
	Limit     int
	// WatchLater: only videos marked 稍后观看.
	WatchLater bool
}

// scanYTChannel fills a channel's nullable columns after a row scan.
func scanYTChannel(c *model.YTChannel, groupID, lastSyncedAt, dateFrom, dateTo sql.NullString, videoCount int) {
	if groupID.Valid {
		c.GroupID = groupID.String
	}
	if lastSyncedAt.Valid {
		v := lastSyncedAt.String
		c.LastSyncedAt = &v
	}
	if dateFrom.Valid {
		v := dateFrom.String
		c.DateFrom = &v
	}
	if dateTo.Valid {
		v := dateTo.String
		c.DateTo = &v
	}
	c.VideoCount = videoCount
}
