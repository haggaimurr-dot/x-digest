package main

import "testing"

func TestOverQuota(t *testing.T) {
	mb := int64(1 << 20)
	if !overQuota(9*mb, 2*mb, 10*mb) {
		t.Fatal("9MB + 2MB over a 10MB quota must be refused")
	}
	if overQuota(8*mb, 2*mb, 10*mb) || overQuota(100*mb, mb, 0) {
		t.Fatal("within quota / no quota must pass")
	}
	t.Setenv("ATTACHMENT_QUOTA_MB", "5")
	if attachmentQuotaBytes() != 5*mb {
		t.Fatal("ATTACHMENT_QUOTA_MB parse")
	}
}
