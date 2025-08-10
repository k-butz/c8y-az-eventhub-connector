#!/bin/bash

# All three arguments can be found as tokens in a Event Hubs connection string
EVENTHUB_URI=$1
SHARED_ACCESS_KEY_NAME=$2
SHARED_ACCESS_KEY=$3
EXPIRY=${EXPIRY:=$((60 * 60 * 24 * 30))} # Default token expiry is 1 day
ENCODED_URI=$(echo -n $EVENTHUB_URI | jq -s -R -r @uri)
TTL=$(($(date +%s) + $EXPIRY))
UTF8_SIGNATURE=$(printf "%s\n%s" $ENCODED_URI $TTL | iconv -t utf8)
HASH=$(echo -n "$UTF8_SIGNATURE" | openssl sha256 -hmac $SHARED_ACCESS_KEY -binary | base64)
ENCODED_HASH=$(echo -n $HASH | jq -s -R -r @uri)
echo -n "SharedAccessSignature sr=$ENCODED_URI&sig=$ENCODED_HASH&se=$TTL&skn=$SHARED_ACCESS_KEY_NAME"
echo ""
