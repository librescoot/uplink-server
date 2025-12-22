#!/bin/bash
# Test script for uplink server REST API

API_KEY="local-dev-api-key"
BASE_URL="http://localhost:8080"

echo "Testing Uplink Server REST API"
echo "==============================="
echo ""

# Test 1: List scooters (should be empty if no scooters connected)
echo "1. Testing GET /api/scooters"
curl -s -H "X-API-Key: $API_KEY" "$BASE_URL/api/scooters" | jq .
echo ""

# Test 2: Try without API key (should fail with 401)
echo "2. Testing authentication (should fail)"
curl -s -w "\nHTTP Status: %{http_code}\n" "$BASE_URL/api/scooters" | head -1
echo ""

# Test 3: Try with wrong API key (should fail with 401)
echo "3. Testing wrong API key (should fail)"
curl -s -w "\nHTTP Status: %{http_code}\n" -H "X-API-Key: wrong-key" "$BASE_URL/api/scooters" | head -1
echo ""

# Test 4: Send command to non-existent scooter (should fail with 404)
echo "4. Testing send command to non-existent scooter (should fail)"
curl -s -X POST -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"scooter_id":"NONEXISTENT","command":"lock","params":{}}' \
  "$BASE_URL/api/commands" | jq .
echo ""

# Test 5: Check command response for non-existent request (should return pending)
echo "5. Testing get command response for non-existent request"
curl -s -H "X-API-Key: $API_KEY" "$BASE_URL/api/commands/20250101-000000.000000" | jq .
echo ""

# Test 6: Get details for non-existent scooter (should fail with 404)
echo "6. Testing get scooter details for non-connected scooter"
curl -s -H "X-API-Key: $API_KEY" "$BASE_URL/api/scooters/NONEXISTENT" | jq .
echo ""

echo "==============================="
echo "Tests complete!"
echo ""
echo "To test with a real scooter:"
echo "1. Connect a scooter client via WebSocket"
echo "2. Run: curl -H 'X-API-Key: $API_KEY' $BASE_URL/api/scooters"
echo "3. Send command: curl -X POST -H 'X-API-Key: $API_KEY' -H 'Content-Type: application/json' \\"
echo "   -d '{\"scooter_id\":\"WUNU2S3B7MZ000147\",\"command\":\"lock\",\"params\":{}}' \\"
echo "   $BASE_URL/api/commands"
