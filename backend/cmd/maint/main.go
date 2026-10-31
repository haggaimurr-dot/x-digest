// Command maint runs the x-digest housekeeping jobs from cron / a systemd
// timer on the server (not exposed over HTTP).
//
//	xdigest-maint -dsn DSN purge-translations [-days 180]
//	    delete cached translations older than N days (translations table)
//	xdigest-maint -dsn DSN purge-categories [-days 7]
//	    hard-delete categories soft-deleted more than N days ago (the API
//	    also does this daily)
//	xdigest-maint -dsn DSN clean-attachments [-dir DIR]
//	    delete files in the attachment directory that no attachments row
//	    points at (default dir $XDIGEST_ATTACHMENT_DIR)
//
// -dsn defaults to $XDIGEST_DSN.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"xdigest/internal/store"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("XDIGEST_DSN"), "MariaDB DSN")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: xdigest-maint [-dsn DSN] purge-translations|purge-categories [-days N] | clean-attachments [-dir DIR]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	cmd, args := flag.Arg(0), flag.Args()[1:]
	if err := run(*dsn, cmd, args); err != nil {
		log.Fatalf("%s: %v", cmd, err)
	}
}

func run(dsn, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	var days int
	switch cmd {
	case "purge-translations":
		fs.IntVar(&days, "days", 180, "keep translations newer than this many days")
	case "purge-categories":
		fs.IntVar(&days, "days", 7, "purge categories deleted more than this many days ago")
	case "clean-attachments":
		days = 1 // unused; files younger than an hour are always kept
	default:
		return fmt.Errorf("unknown command (want purge-translations, purge-categories or clean-attachments)")
	}
	dir := fs.String("dir", envOr("XDIGEST_ATTACHMENT_DIR", "/var/lib/xdigest/attachments"), "attachment directory (clean-attachments)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if days < 1 {
		return fmt.Errorf("-days must be >= 1")
	}
	if dsn == "" {
		return fmt.Errorf("no DSN: pass -dsn or set XDIGEST_DSN")
	}
	st, err := store.New(dsn)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	var n int64
	switch cmd {
	case "purge-translations":
		n, err = st.PurgeTranslations(days)
	case "purge-categories":
		n, err = st.PurgeDeletedCategories(days)
	case "clean-attachments":
		known, kerr := st.AttachmentFileNames()
		if kerr != nil {
			return kerr
		}
		removed, cerr := store.CleanOrphanFiles(*dir, known, time.Hour, time.Now())
		n, err = int64(len(removed)), cerr
	}
	if err != nil {
		return err
	}
	log.Printf("%s: %d rows removed (older than %d days)", cmd, n, days)
	return nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
