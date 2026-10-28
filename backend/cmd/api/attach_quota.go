package main

import (
	"log"
	"os"
	"time"

	"xdigest/internal/store"
)

// Attachment storage limits (2026-10-28).
//
//	ATTACHMENT_QUOTA_MB  total size of all attachments; an upload that would
//	                     go over answers 413 QUOTA_EXCEEDED. 0/unset = none.
//
// Orphan files (no attachments row; see store.CleanOrphanFiles) are removed
// at start-up and daily; `xdigest-maint clean-attachments` does it by hand.

func attachmentQuotaBytes() int64 {
	return int64(atoiDefault(os.Getenv("ATTACHMENT_QUOTA_MB"), 0)) << 20
}

// overQuota: would adding incoming bytes to used exceed quota (0 = none)?
func overQuota(used, incoming, quota int64) bool {
	return quota > 0 && used+incoming > quota
}

const orphanMinAge = time.Hour

func (s *server) cleanAttachmentsLoop() {
	for {
		known, err := s.st.AttachmentFileNames()
		if err != nil {
			log.Printf("attachment cleanup: %v", err)
		} else if removed, err := store.CleanOrphanFiles(s.attachDir, known, orphanMinAge, time.Now()); err != nil {
			log.Printf("attachment cleanup: %v", err)
		} else if len(removed) > 0 {
			log.Printf("attachment cleanup: removed %d orphan files", len(removed))
		}
		time.Sleep(24 * time.Hour)
	}
}
