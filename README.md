# Sportsbook EDA (tutorial)

Event-driven slice of a sportsbook stack in Go: **MySQL** (bets, wallets, transactional outbox), **RabbitMQ** (topic exchange, queues), **stdlib `net/http`**, and **Docker Compose**. 

This implementation demonstrates a robust **Choreography** pattern with high observability, idempotency, and business-aware validation (drift protection and market suspension).

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (or Docker Engine + Compose v2) with the daemon running
- [Go 1.22+](https://go.dev/dl/) if you want to run or test binaries on the host
- [Postman](https://www.postman.com/downloads/) for the interactive demo

## Quick Start (Full Stack)

From the repo root, you can use the **Makefile** or the automation **scripts**:

```bash
# Option 1: Using Makefile
make up

# Option 2: Using Bash Script
./scripts/run.sh
```

- **Gateway (Edge):** http://localhost:8080  
- **Health:** `GET http://localhost:8080/healthz`  
- **RabbitMQ Management UI:** http://localhost:15672 (guest / guest)
- **Metrics:** `GET http://localhost:9090/debug/vars` (Gateway JSON)

### Automation Scripts (`scripts/`)
| Script | Purpose |
|--------|---------|
| `run.sh` | Builds and starts the entire stack in the background. |
| `stop.sh` | Safely halts services (graceful shutdown). |
| `clean.sh` | Wipes database volumes and project-specific images. |
| `seed_odds.sh` | Feeds 100 random market odds into the dynamic odds-service. |

## Interactive Demo (Postman)

A guided Postman collection is available at the project root: 
`postman_collection.json`

### Demo Steps:
1.  **Import** the collection into Postman.
2.  **Select/Create an Environment** named `Sportsbook`.
3.  **Place Bet**: Run "Place Bet (Success - User 1)". The script automatically captures `bet_id` and `correlation_id`.
4.  **Observe**: Check logs to see the choreography (Bet -> Wallet -> Accept).
5.  **Settle**: Run "Settle Bet (WIN)". It uses the captured IDs to credit the wallet.

## Key EDA Features

- **Transactional Outbox**: All state changes and their corresponding events are committed atomically to MySQL.
- **Reliable Delivery**: The `outbox-relay` ensures at-least-once delivery to RabbitMQ.
- **Idempotency**: All consumers use a `processed_events` table to prevent duplicate processing.
- **Live Validation**: The `bet-service` performs live checks against the `odds-service` for:
    - **Market Suspension**: Rejects bets if the market status is not `OPEN`.
    - **Odds Drift**: Rejects bets if the user's `odds_version` is stale.

## Operations & Debugging

### Logging
Monitor the entire distributed flow in one window:
```bash
make logs
```

### Database Inspection
Check the real-time status of bets or user balances:
```bash
make check-bets
make check-wallets
```

### Manual Seed
If you need to reset or add extra test users manually:
```bash
docker compose exec -T mysql mysql -usportsbook -psportsbook sportsbook < internal/migrate/sql/manual_seed.sql
```

## Service Map

| Service | Responsibility |
|---------|----------------|
| `api-gateway` | Edge proxy + Error handling (502 diagnostics) |
| `bet-service` | Bet placement + live validation (Suspension/Drift) |
| `outbox-relay` | Transactional outbox polling & reliable publishing |
| `wallet-service` | Stake reservation & settlement credits (Idempotent) |
| `bet-worker` | Bet acceptance choreography |
| `risk-service` | Async risk evaluation hook |
| `odds-service` | **Dynamic** odds store (In-memory, supports `POST`) |
| `notification-service` | Event subscriber (Accepted/Settled) |
| `migrate` | One-shot migration runner (serialized via `GET_LOCK`) |

## Production Readiness & Advanced Topics

This project is a functional slice designed for learning and demonstration. For a deep dive into how this architecture scales to 100k+ users and handles high-burst scenarios (like the World Cup), see the **[Architecture Deep-Dive](docs/ARCHITECTURE.md)**.

**Topics covered in the deep-dive:**
- **Transactional Outbox & CDC**: How we solve the "Dual Write" problem.
- **Idempotency Strategy**: Ensuring "Exactly-Once" business logic in an "At-Least-Once" network.
- **Scaling the Relay**: Using `SKIP LOCKED` for parallel event publishing.
- **Distributed Sagas**: Handling partial failures with compensating transactions.
- **Resilience**: Dead Letter Queues (DLQ) and Graceful Shutdown patterns.

## License
Educational / A step before production grade.
