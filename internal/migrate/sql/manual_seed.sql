-- Dev seed (run manually after migrations). Example:
--   docker compose exec -T mysql mysql -usportsbook -psportsbook sportsbook < scripts/dev-seed.sql
-- Or from host with client on PATH:
--   mysql -h127.0.0.1 -P3306 -usportsbook -psportsbook sportsbook < scripts/dev-seed.sql

INSERT IGNORE INTO users (id) VALUES ('00000000-0000-0000-0000-000000000001');

INSERT IGNORE INTO wallets (user_id, available_amount, currency, version)
VALUES ('00000000-0000-0000-0000-000000000001', 10000.0000, 'USD', 0);
