-- Initial schema: users, wallets, bets, outbox, idempotency.
-- Applied once per environment by internal/migrate (see schema_migrations).

CREATE TABLE users (
  id CHAR(36) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (id)
);

CREATE TABLE wallets (
  user_id CHAR(36) NOT NULL,
  available_amount DECIMAL(19,4) NOT NULL DEFAULT 0.0000,
  currency CHAR(3) NOT NULL DEFAULT 'USD',
  version INT UNSIGNED NOT NULL DEFAULT 0,
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (user_id),
  CONSTRAINT fk_wallets_user FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE TABLE bets (
  id CHAR(36) NOT NULL,
  user_id CHAR(36) NOT NULL,
  stake_amount DECIMAL(19,4) NOT NULL,
  currency CHAR(3) NOT NULL,
  selection_id VARCHAR(128) NOT NULL,
  market_id VARCHAR(128) NOT NULL,
  odds DECIMAL(10,4) NOT NULL,
  odds_version INT NOT NULL,
  status VARCHAR(32) NOT NULL,
  correlation_id CHAR(36) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (id),
  CONSTRAINT fk_bets_user FOREIGN KEY (user_id) REFERENCES users (id),
  KEY idx_bets_user (user_id),
  KEY idx_bets_correlation (correlation_id)
);

CREATE TABLE outbox_events (
  id BIGINT NOT NULL AUTO_INCREMENT,
  event_type VARCHAR(128) NOT NULL,
  routing_key VARCHAR(256) NOT NULL,
  payload_json JSON NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  published_at DATETIME(6) NULL DEFAULT NULL,
  aggregate_id CHAR(36) NULL,
  PRIMARY KEY (id),
  KEY idx_outbox_unpublished (published_at, id)
);

CREATE TABLE processed_events (
  event_id CHAR(36) NOT NULL,
  consumer_name VARCHAR(128) NOT NULL,
  processed_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (event_id, consumer_name)
);
