-- Idempotent dev seed (INSERT IGNORE). Safe to re-apply via migration version tracking.

-- User 1: Sufficient funds (10,000 USD)
INSERT IGNORE INTO users (id) VALUES ('00000000-0000-0000-0000-000000000001');
INSERT IGNORE INTO wallets (user_id, available_amount, currency, version)
VALUES ('00000000-0000-0000-0000-000000000001', 10000.0000, 'USD', 0);

-- User 2: Low funds (5.00 USD)
INSERT IGNORE INTO users (id) VALUES ('00000000-0000-0000-0000-000000000002');
INSERT IGNORE INTO wallets (user_id, available_amount, currency, version)
VALUES ('00000000-0000-0000-0000-000000000002', 5.0000, 'USD', 0);

-- User 3: Different currency (500.00 EUR)
INSERT IGNORE INTO users (id) VALUES ('00000000-0000-0000-0000-000000000003');
INSERT IGNORE INTO wallets (user_id, available_amount, currency, version)
VALUES ('00000000-0000-0000-0000-000000000003', 500.0000, 'EUR', 0);
