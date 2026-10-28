// Command xdigest-import imports a legacy x-digest static site into MariaDB.
//
// Usage (run on the server only; there is deliberately no HTTP import endpoint):
//
//	xdigest-import -data-dir <old site root> -dsn <DSN> [-report report.json]
//
// It prints a JSON report (counts plus inserted / updated / skipped / failed
// posts, with the id and reason of each failure) and also writes it to
// -report when given. Posts that fail validation are left out and the rest
// still go in; the exit code is then 1, so a cron job / CI notices.
//
// The import is idempotent: posts/steps/tags are replaced per post and all
// other rows are upserted by primary key, so re-running it does not duplicate.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"

	"xdigest/internal/importer"
)

func main() {
	dataDir := flag.String("data-dir", "", "legacy x-digest site root (with data/posts.json, config/*.json)")
	dsn := flag.String("dsn", "", "MySQL DSN, e.g. user:pass@tcp(127.0.0.1:3306)/xdigest")
	reportPath := flag.String("report", "", "also write the JSON report to this file")
	flag.Parse()
	if *dataDir == "" || *dsn == "" {
		fmt.Fprintln(os.Stderr, "usage: xdigest-import -data-dir <old site root> -dsn <DSN>")
		os.Exit(2)
	}
	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}
	counts, err := importer.Import(db, *dataDir)
	if err != nil {
		log.Fatalf("import failed: %v", err)
	}
	report, _ := json.MarshalIndent(counts, "", "  ")
	fmt.Println(string(report))
	if *reportPath != "" {
		if err := os.WriteFile(*reportPath, append(report, '\n'), 0o644); err != nil {
			log.Printf("write report: %v", err)
		}
	}
	if len(counts.Failed) > 0 {
		fmt.Fprintf(os.Stderr, "import finished with %d failed posts\n", len(counts.Failed))
		os.Exit(1)
	}
	fmt.Println("import ok")
}
