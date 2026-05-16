# Change Data Capture (CDC) Implementation & Demo

This document explains the **Change Data Capture (CDC)** implementation in the Sportsbook platform. We have replaced the traditional polling-based Outbox Relay with a near-instant binlog listener to achieve high-performance event streaming.

## 1. Why CDC?

In the original implementation, the `outbox-relay` polled the `outbox_events` table every 500ms. While simple, this had two drawbacks:
1.  **Latency:** Events could sit in the database for up to 500ms before being published.
2.  **DB Load:** Constant `SELECT ... FOR UPDATE` queries create unnecessary load and lock contention on the database, especially as volume scales.

**CDC solves this** by streaming changes directly from the MySQL Binary Log (binlog) as they happen.

---

## 2. Technical Stack

-   **Library:** [`github.com/go-mysql-org/go-mysql`](https://github.com/go-mysql-org/go-mysql)
-   **MySQL Configuration:**
    -   `binlog-format=ROW`: Captures the actual data changes in each row.
    -   `binlog-row-image=FULL`: Ensures the full before/after state is available.
- **Service:** `outbox-relay` (running in CDC mode via `USE_CDC=1`).

---

## 2.1 Prerequisites & Configuration

For CDC to work properly, the following are required:
1.  **MySQL Privileges:** The database user requires `REPLICATION SLAVE` and `REPLICATION CLIENT` permissions. These are automatically applied during the `migrate` step in our Docker Compose environment (which runs migrations as `root`).
2.  **Binlog Format:** MySQL must be configured with `binlog-format=ROW`.
3.  **DSN Connection:** The `outbox-relay` automatically parses the `MYSQL_DSN` to establish the binlog connection.

---

## 3. How it Works


1.  **Transactional Write:** The `bet-service` saves a bet and an outbox event in a single MySQL transaction.
2.  **Binlog Emission:** Once the transaction commits, MySQL writes the new row to its internal Binary Log.
3.  **Real-time Capture:** The `outbox-relay` acts as a MySQL "Slave" (using the `canal` package). it receives the binlog stream via the MySQL replication protocol.
4.  **Event Filtering:** The relay filters the stream specifically for `INSERT` operations on the `sportsbook.outbox_events` table.
5.  **Instant Publish:** The relay extracts the `routing_key` and `payload_json` from the binlog event and publishes them to RabbitMQ immediately.

---

## 4. Demo Instructions

Follow these steps to demonstrate the CDC relay in action.

### Step 1: Start the Stack
Ensure the environment is fresh and running:
```bash
./scripts/clean.sh  # Optional: Wipe old data
./scripts/run.sh
```

### Step 2: Observe the CDC Relay Initialization
Check the logs of the `outbox-relay` to see it connecting to the MySQL binlog:
```bash
docker compose logs -f outbox-relay
```
*You should see: `starting CDC outbox relay (binlog listener)`*

### Step 3: Trigger an Event
In a new terminal, place a bet via the API Gateway:
```bash
curl -s -X POST http://localhost:8080/bets \
    -H "Content-Type: application/json" \
    -d '{
        "user_id": "00000000-0000-0000-0000-000000000001",
        "stake_amount": "10.00",
        "currency": "USD",
        "selection_id": "sel_demo_1",
        "market_id": "mkt_demo_1",
        "odds": 1.95,
        "odds_version": 99
    }'
```

### Step 4: Verify Instant Delivery
Look back at the `outbox-relay` logs. You will see the events being published **instantly** without the 500ms polling delay:
```text
{"level":"INFO","msg":"CDC: outbox event published","routing_key":"bet.placed.v1"}
{"level":"INFO","msg":"CDC: outbox event published","routing_key":"wallet.reserved.v1"}
{"level":"INFO","msg":"CDC: outbox event published","routing_key":"bet.accepted.v1"}
```

---

## 5. Automated Verification
We have provided a script that automates the verification of both CDC and Redis caching:
```bash
bash scripts/verification.sh
```

## 6. Key Code References
- **Configuration:** `docker-compose.yml` (MySQL `command` flags).
- **Relay Logic:** `internal/outboxrelay/cdc.go` (The `OnRow` event handler).
- **Entry Point:** `cmd/outbox-relay/main.go` (Toggles between Polling and CDC).
