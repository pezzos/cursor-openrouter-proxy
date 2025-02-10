#!/bin/bash

# test_proxy.sh
echo "Testing Cursor Proxy..."

response=$(curl -s -w "\n%{http_code}" https://cursor-proxy.home.pezzos.com/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-or-v1-1234567890" \
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
    echo -e "\n❌ Test failed!"
    exit 1
else
    echo -e "\n✅ Test successful!"
fi
