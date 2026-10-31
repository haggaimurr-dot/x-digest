package main

import (
	"strings"
	"testing"
)

func TestRunValidation(t *testing.T) {
	for _, c := range []struct {
		cmd  string
		args []string
		dsn  string
		want string
	}{
		{"bogus", nil, "x", "unknown command"},
		{"purge-translations", []string{"-days", "0"}, "x", "-days must be >= 1"},
		{"purge-translations", nil, "", "no DSN"},
	} {
		err := run(c.dsn, c.cmd, c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("run(%q, %v): %v, want %q", c.cmd, c.args, err, c.want)
		}
	}
}
