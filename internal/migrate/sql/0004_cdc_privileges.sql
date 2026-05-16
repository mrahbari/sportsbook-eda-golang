-- Grant CDC privileges to the application user.
-- This requires the migration to be run with a user that has GRANT OPTION (e.g., root).
-- In local development (Docker), we ensure the migrate service has root access.

GRANT REPLICATION SLAVE, REPLICATION CLIENT ON *.* TO 'sportsbook'@'%';
FLUSH PRIVILEGES;
