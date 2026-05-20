#!/bin/bash

# test_proxy.sh
echo "Testing Cursor Proxy..."

PROXY_URL="${PROXY_URL:-http://127.0.0.1:9000}"

# Use a fake API key because it's rewritten by the proxy
response=$(curl -s -w "\n%{http_code}" "$PROXY_URL/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer local-test-key" \
  -d '{
    "messages": [
      {
        "role": "system",
        "content": "You are a test assistant."
      },
      {
        "role": "user",
        "content": "Testing. Just say hi and nothing else."
      }
    ],
    "model": "gpt-4o"
  }')

# Extract status code
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

echo -e "\nStatus code: $http_code"
echo -e "\nResponse body:"
echo "$body" | jq '.' 2>/dev/null || echo "$body"

if [ "$http_code" -ne 200 ]; then
    echo -e "\nTest failed!"
    exit 1
else
    echo -e "\nTest successful!"
fi
