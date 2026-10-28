#!/bin/bash
# Daily backup of the x-digest database and attachments (2026-10-28).
# Run by xdigest-backup.timer as root. Keeps 14 days.
#
#   BACKUP_DIR  default /var/backups/xdigest
#   DB name/user/password come from the DSN in /etc/xdigest/api.env
#   (XDIGEST_DSN=user:pass@tcp(host:port)/db...), so no password is stored
#   in this script or on the command line (it goes through a 600 defaults
#   file that is deleted afterwards).
set -euo pipefail
BACKUP_DIR=${BACKUP_DIR:-/var/backups/xdigest}
KEEP_DAYS=${KEEP_DAYS:-14}
ATTACH_DIR=$(sed -n 's/^XDIGEST_ATTACHMENT_DIR=//p' /etc/xdigest/api.env | tail -1)
ATTACH_DIR=${ATTACH_DIR:-/var/lib/xdigest/attachments}
DSN=$(sed -n 's/^XDIGEST_DSN=//p' /etc/xdigest/api.env | tail -1)
# user:pass@tcp(host:port)/db?params
DB_USER=${DSN%%:*}
rest=${DSN#*:}
DB_PASS=${rest%%@tcp(*}
hostport=${rest#*@tcp(}; hostport=${hostport%%)*}
DB_HOST=${hostport%%:*}
DB_PORT=${hostport##*:}
db=${rest#*)/}; DB_NAME=${db%%\?*}

stamp=$(date +%Y%m%d-%H%M%S)
mkdir -p "$BACKUP_DIR/db" "$BACKUP_DIR/attachments"
chmod 700 "$BACKUP_DIR"
cnf=$(mktemp)
trap 'rm -f "$cnf"' EXIT
chmod 600 "$cnf"
printf '[client]\nuser=%s\npassword=%s\nhost=%s\nport=%s\n' "$DB_USER" "$DB_PASS" "$DB_HOST" "$DB_PORT" > "$cnf"

mysqldump --defaults-extra-file="$cnf" --single-transaction --routines --triggers "$DB_NAME" \
  | gzip -9 > "$BACKUP_DIR/db/xdigest-$stamp.sql.gz"
# Attachments: a mirror (rsync, deletions kept out of it for 14 days via
# --backup-dir), cheap because files never change after upload.
rsync -a --delete --backup --backup-dir="$BACKUP_DIR/attachments-deleted/$stamp" \
  "$ATTACH_DIR/" "$BACKUP_DIR/attachments/"

find "$BACKUP_DIR/db" -name 'xdigest-*.sql.gz' -mtime +"$KEEP_DAYS" -delete
find "$BACKUP_DIR/attachments-deleted" -mindepth 1 -maxdepth 1 -mtime +"$KEEP_DAYS" -exec rm -rf {} + 2>/dev/null || true
echo "backup ok: $BACKUP_DIR/db/xdigest-$stamp.sql.gz"
