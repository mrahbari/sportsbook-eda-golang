#!/bin/bash
set -e

GATEWAY_URL="http://localhost:8080"

echo "🔍 Verifying Redis Caching in Odds Service..."
# First request: Cache MISS
echo "Request 1 (MISS expected):"
curl -v "$GATEWAY_URL/v1/markets/mkt_demo_1/odds" 2>&1 | grep -E "X-Cache|market_id"

# Second request: Cache HIT
echo "Request 2 (HIT expected):"
curl -v "$GATEWAY_URL/v1/markets/mkt_demo_1/odds" 2>&1 | grep -E "X-Cache|market_id"

echo -e "\n🔍 Verifying CDC Relay in Outbox Service..."
# Place a bet to trigger an outbox event
echo "Placing a bet..."
BET_PAYLOAD='{
    "user_id": "00000000-0000-0000-0000-000000000001",
    "stake_amount": "10.00",
    "currency": "USD",
    "selection_id": "sel_demo_1",
    "market_id": "mkt_demo_1",
    "odds": 1.95,
    "odds_version": 99
}'

curl -s -X POST "$GATEWAY_URL/bets" \
    -H "Content-Type: application/json" \
    -d "$BET_PAYLOAD"

echo -e "\nChecking outbox-relay logs for CDC activity..."
docker compose logs outbox-relay | tail -n 20
