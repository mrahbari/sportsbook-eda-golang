#!/bin/bash

# Seeding 100 random market odds for testing
# Usage: ./scripts/seed_odds.sh [gateway_url]

GATEWAY_URL=${1:-"http://localhost:8080"}

echo "🎲 Seeding 100 random market odds to $GATEWAY_URL..."

for i in {1..100}
do
  MARKET_ID="mkt_stress_$i"
  SELECTION_ID="sel_stress_$i"
  # Generate a random price between 1.10 and 5.00
  PRICE=$(printf "%.2f" $(echo "scale=2; 1.1 + (4.9 * $RANDOM / 32767)" | bc))
  VERSION=$(( ( RANDOM % 100 )  + 1 ))

  JSON_BODY=$(cat <<EOF
{
  "odds_version": $VERSION,
  "status": "OPEN",
  "selections": [
    {
      "selection_id": "$SELECTION_ID",
      "price": $PRICE
    }
  ]
}
EOF
)

  curl -s -X POST "$GATEWAY_URL/v1/markets/$MARKET_ID/odds" \
    -H "Content-Type: application/json" \
    -d "$JSON_BODY" > /dev/null

  if [ $((i % 10)) -eq 0 ]; then
    echo "✅ Seeded $i/100 markets..."
  fi
done

echo "🎉 Finished seeding 100 random odds!"
echo "Try fetching one: curl -s $GATEWAY_URL/v1/markets/mkt_stress_50/odds"
