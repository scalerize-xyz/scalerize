#!/bin/sh

sleep 5
IFS=',' read -r -a urls <<< "$PEER_RPC_URLS"

enode_urls=()

add_enode() {
  local url=$1
  local retries=3
  local delay=3

  for ((i = 1; i <= retries; i++)); do
    response=$(curl -s -X POST --data '{"jsonrpc":"2.0","method":"admin_nodeInfo","params":[],"id":1}' \
      -H "Content-Type: application/json" "$url")

    echo "Response from $url:"
    echo "$response"

    enode=$(echo "$response" | jq -r '.result.enode')

    if [[ -n "$enode" ]]; then
      echo "Found enode: $enode"
      enode_urls+=("$enode")
      return 0 
    fi

    echo "No valid response, attempt $i of $retries. Retrying in $delay seconds..."
    sleep $delay
  done

  echo "Failed to get a valid response from $url after $retries attempts."
  return 1 
}

add_trusted_peer() {
  local enode=$1
  response=$(curl -s -X POST --data "{\"jsonrpc\":\"2.0\",\"method\":\"admin_addTrustedPeer\",\"params\":[\"$enode\"],\"id\":1}" \
    -H "Content-Type: application/json" "http://localhost:8545")

  if echo "$response" | jq -e '.result == true'; then
    echo "Successfully added $enode as a trusted peer."
  else
    echo "Failed to add $enode as a trusted peer."
  fi
}

for url in "${urls[@]}"; do
  add_enode "$url"
done

for enode_url in "${enode_urls[@]}"; do
  add_trusted_peer "$enode_url"  
done
