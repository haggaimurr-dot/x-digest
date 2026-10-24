#!/bin/bash
# Deploy the x-digest NEW stack on the Shanghai box (run there via sudo bash).
# This script only installs/starts services from files already copied to
# /opt/xdigest by the VM side (binaries + Nuxt .output). It never creates
# passwords: /etc/xdigest/api.env (root-only, 600) must already exist with
# XDIGEST_ADDR / XDIGEST_DSN / XDIGEST_ATTACHMENT_DIR, and the xdigest
# MariaDB schema + user must already exist (backend/schema.sql, db-user.sql).
# It does NOT touch the 18089 nginx vhost — switching is a separate,
# deliberate step (see README rollback section).
set -euo pipefail
mkdir -p /opt/xdigest/backend /opt/xdigest/frontend /var/lib/xdigest/attachments /etc/xdigest
chown -R ubuntu:ubuntu /opt/xdigest /var/lib/xdigest
chmod 755 /opt/xdigest/backend/xdigest-api /opt/xdigest/backend/xdigest-import
cp /opt/xdigest/backend/xdigest-api.service /etc/systemd/system/xdigest-api.service
cp /opt/xdigest/frontend/xdigest-web.service /etc/systemd/system/xdigest-web.service
test -f /etc/xdigest/api.env
chmod 600 /etc/xdigest/api.env
systemctl daemon-reload
systemctl enable --now xdigest-api
systemctl enable --now xdigest-web
sleep 3
curl -s -m 5 http://127.0.0.1:18090/healthz; echo " <- api healthz"
curl -s -m 5 -o /dev/null -w "web %{http_code}\n" http://127.0.0.1:18091/
systemctl is-active xdigest-api xdigest-web
