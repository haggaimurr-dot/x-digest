-- Dedicated application user for the xdigest database.
-- TEMPLATE ONLY: replace XDIGEST_DB_PASSWORD with a real password when
-- applying on the server, and store it only in the root-only
-- /etc/xdigest/api.env (mode 600). Never commit a real password.
-- Apply as a privileged user:  mysql < db-user.sql

CREATE USER IF NOT EXISTS 'xdigest'@'127.0.0.1' IDENTIFIED BY 'XDIGEST_DB_PASSWORD';
GRANT SELECT, INSERT, UPDATE, DELETE ON xdigest.* TO 'xdigest'@'127.0.0.1';
FLUSH PRIVILEGES;
