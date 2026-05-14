# Sportsbook Platform Architecture

> **Domain:** Distributed Systems, High-Frequency Betting, Event-Driven Architecture (EDA)  
> **Stack:** Go (Golang), RabbitMQ, MySQL, Docker

---

## 1. Executive Summary

This project demonstrates a production-grade **Event-Driven Architecture (EDA)** for a Sportsbook platform. The system is designed to handle the core betting lifecycle—placement, validation, reservation, and acceptance—using asynchronous communication to ensure high availability and data integrity.

By leveraging **RabbitMQ** as a message broker and the **Transactional Outbox Pattern**, we achieve a decoupled, resilient ecosystem where financial consistency is guaranteed even in the face of partial system failures.

---

## 2. High-Level Architecture

The system follows a microservices pattern where services communicate asynchronously via RabbitMQ Topic Exchanges. Each service is responsible for its own domain logic and maintains its own state in a shared MySQL instance (partitioned by table prefix for simplicity in this demonstration).

### System Diagram

```text
                                  [ External Odds Provider ]
                                            │
                                            ▼
┌────────────────┐          ┌──────────────────────────────────┐          ┌──────────────────┐
│  API Gateway   │          │          Odds Service            │          │  Notification    │
│  (Auth/Entry)  │          │  (Market State/Price Updates)    │          │     Service      │
└───────┬────────┘          └───────────────┬──────────────────┘          └────────▲─────────┘
        │                                   │                                      │
        ▼                                   ▼                                      │
┌────────────────┐          ┌──────────────────────────────────┐          ┌────────┴─────────┐
│  Bet Service   │          │        RabbitMQ Cluster          │          │   WebSockets /   │
│ (Orchestrator) │◄────────►│   (Exchanges, Queues, DLQs)      │◄────────►│   Push Engine    │
└───────┬────────┘          └───────▲───────────────▲──────────┘          └──────────────────┘
        │                           │               │
        ▼                           ▼               ▼
┌────────────────┐          ┌────────────────┐  ┌────────────────┐
│ Wallet Service │          │ Settlement Svc │  │ Risk/Analytics │
│ (Accounting)   │          │ (Resulting)    │  │ (Async Check)  │
└────────────────┘          └────────────────┘  └────────────────┘
```

---

## 3. The Lifecycle of a Bet (Event Flow)

1.  **Placement:** User submits a bet slip via the **API Gateway** to the **Bet Service**.
2.  **Validation:** The **Bet Service** performs a synchronous check against the **Odds Service** to ensure the market is open and odds haven't drifted beyond tolerance.
3.  **Persistence:** If valid, the **Bet Service** saves the bet as `PENDING_RESERVE` and records a `bet.placed.v1` event in its **Transactional Outbox**.
4.  **Relay:** The **Outbox Relay** polls the outbox and publishes the event to **RabbitMQ**.
5.  **Reservation:** The **Wallet Service** consumes `bet.placed.v1`, reserves the funds from the user's balance, and emits `wallet.reserved.v1`.
6.  **Acceptance:** The **Bet Service** (via `bet-worker`) consumes `wallet.reserved.v1`, moves the bet to `OPEN`, and emits `bet.accepted.v1`.
7.  **Analytics:** The **Risk Service** consumes `bet.placed.v1` asynchronously for background fraud detection and risk scoring.
8.  **Notification:** The **Notification Service** consumes acceptance and settlement events to keep the user informed.

---

## 4. RabbitMQ Topology Design

We utilize **Topic Exchanges** for maximum routing flexibility and **Dead Letter Exchanges (DLX)** for robust error handling.

### Naming Conventions
- **Exchanges:** `xc.<domain>.<type>` (e.g., `xc.betting.topic`)
- **Queues:** `q.<consumer-service>.<event-type>` (e.g., `q.wallet.bet-placed`)
- **Routing Keys:** `<entity>.<action>.<version>` (e.g., `bet.placed.v1`)

### Topology Mapping
| Exchange | Routing Key | Queue | Consumer |
| :--- | :--- | :--- | :--- |
| `xc.betting.topic` | `bet.placed.*` | `q.wallet.bet-placed` | Wallet Service |
| `xc.betting.topic` | `bet.placed.*` | `q.risk.bet-placed` | Risk Service |
| `xc.betting.topic` | `wallet.reserved.*` | `q.bet.wallet-reserved` | Bet Service (Worker) |
| `xc.betting.topic` | `bet.accepted.*` | `q.notify.bet-events` | Notification Service |

---

## 5. Event Schema Design

All events follow a strict, versioned envelope structure to ensure backward compatibility and traceability.

### Standard Envelope (`EventMetadata`)
Every event carries metadata for distributed tracing and deduplication:
- `event_id`: UUID for idempotency checks.
- `correlation_id`: Tracks a business operation across all services.
- `causation_id`: The ID of the event that triggered this operation.
- `producer`: The service that generated the event.

### Sample Event: `bet.placed.v1`
```json
{
  "metadata": {
    "event_id": "evt_789abc",
    "correlation_id": "corr_123xyz",
    "causation_id": "req_456def",
    "producer": "bet-service",
    "version": "v1"
  },
  "payload": {
    "bet_id": "bet_001",
    "user_id": "user_99",
    "amount": "100.00",
    "currency": "USD",
    "odds": 1.95
  }
}
```

---

## 6. Distributed Data Integrity

### Transactional Outbox Pattern
To solve the **"Dual Write" problem** (writing to DB and publishing to MQ), we use the Outbox Pattern:
1. **Atomic Write:** The business state and the event are saved in the same database transaction.
2. **Asynchronous Publishing:** A dedicated `outbox-relay` service polls the `outbox_events` table and publishes messages to RabbitMQ only after the DB transaction is committed.
3. **Reliability:** If the relay crashes, it resumes from the last unpublished event, ensuring **At-Least-Once Delivery**.

### Idempotency Strategy
Since messages can be redelivered (At-Least-Once), every consumer is **idempotent**. 
- Services maintain a `processed_events` table.
- Before processing, they check if `(event_id, consumer_name)` already exists.
- This prevents duplicate processing of the same event (e.g., double-debiting a wallet).

---

## 7. Resilience & Fault Tolerance

### Graceful Shutdown
All Go services implement graceful shutdown using `context` and `sync.WaitGroup`. When a service receives a `SIGTERM`:
1. It stops accepting new messages from RabbitMQ.
2. It waits for in-flight processing to complete.
3. It acknowledges processed messages before exiting.

### Dead Letter Queues (DLQ)
Messages that fail repeatedly or are "poison" (unparseable) are moved to the `xc.betting.dlx` exchange and stored in `.dlq` queues for manual inspection and replay, preventing them from blocking the main processing pipeline.

---

## 8. Observability

- **Correlation IDs:** Every log line and event carries a `correlation_id`, allowing us to trace a single bet slip from the gateway through all downstream services.
- **Structured Logging:** Uses `slog` (JSON) for easy ingestion into logging platforms like ELK or Loki.
- **Metrics:** Basic instrumentation for outbox throughput and consumer processing rates.

---

## 9. Future Production Improvements

- **Change Data Capture (CDC):** Replace polling with a tool like **Debezium** to read MySQL binlogs for lower latency.
- **Redis Caching:** Use Redis for hot market data (odds) to reduce MySQL read load.
---

## 10. Advanced System Design Scenarios

To move this system from a demonstration to a high-scale production environment (10k+ bets/sec), the following advanced patterns should be considered:

### 10.1 Scaling the Outbox Relay (Parallelism)
The current `outbox-relay` processes events sequentially. In a high-volume system, a single poller becomes a bottleneck.
- **Solution:** Deploy multiple relay workers and use **`SELECT ... FOR UPDATE SKIP LOCKED`** (supported in MySQL 8.0+). 
- **Benefit:** This allows multiple workers to grab different batches of unpublished events simultaneously without blocking each other, significantly increasing throughput.

### 10.2 Compensating Transactions (Sagas)
Our "Bet Lifecycle" is a **Choreography-based Saga**. If a step fails late in the chain (e.g., the `bet-worker` fails to move a bet to `OPEN` even though the wallet reserved funds), the system must self-heal.
- **Implementation:** If `bet-worker` detects a terminal failure, it emits a `bet.failed.v1` event.
- **Compensation:** The **Wallet Service** consumes `bet.failed.v1` and executes a "Compensating Transaction" to release the reserved funds back to the user.

### 10.3 Locking Strategy: Optimistic vs. Pessimistic
- **Pessimistic Locking (`FOR UPDATE`):** Used in the **Wallet Service** during reservation to ensure zero double-spending. It's safer but holds DB connections longer.
- **Optimistic Locking (Versioning):** Used in the **Odds Service** and potentially for user profile updates. It uses a `version` column in the `WHERE` clause.
- **Recommendation:** Use Pessimistic locking for high-value financial transactions (Wallets) and Optimistic locking for high-frequency, low-contention updates (Market metadata).

### 10.4 Event Versioning & Schema Registry
As the system evolves, event schemas will change.
- **Backward Compatibility:** Always add fields as optional; never rename or delete fields in a `v1` schema.
- **Side-by-Side Migration:** When a breaking change is required, publish `v1` and `v2` simultaneously during a transition window until all consumers have migrated.
- **Registry:** In production, use a **Schema Registry** (like Confluent or a simple JSON Schema store) to enforce contracts between services at build time.

### 10.5 Performance Tuning for "The Goal Surge"
Sportsbooks face extreme bursts (e.g., a goal in the World Cup).
- **RabbitMQ Prefetch:** Tune the `prefetch_count` (we use 30) based on the processing time of each service to prevent worker starvation or memory exhaustion.
- **Database Connection Pooling:** Ensure `MaxOpenConns` is tuned to the capacity of the MySQL instance, and use a proxy like **ProxySQL** for better load balancing and connection multiplexing.
