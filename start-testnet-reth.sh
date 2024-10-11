ETH_DATA_DIR=.scalerized/eth
JWT_PATH=testing/files/jwt.hex
ETH_GENESIS_PATH=testing/files/eth-genesis.json

IFS=',' read -r -a urls <<< "$PEER_RPC_URLS"

enode_urls=()

for url in "${urls[@]}"; do
  response=$(curl -s -X POST --data '{"jsonrpc":"2.0","method":"admin_nodeInfo","params":[],"id":1}' \
    -H "Content-Type: application/json" "$url")

  echo "Response from $url:"
  echo "$response"

  enode=$(echo "$response" | jq -r '.result.enode')

  if [[ -n "$enode" ]]; then
    enode_urls+=("$enode")
  fi
done

ENODE_URLS=$(IFS=','; echo "${enode_urls[*]}")

export ENODE_URLS

echo "ENODE_URLS=$ENODE_URLS"

mkdir -p $ETH_DATA_DIR
reth init --datadir $ETH_DATA_DIR --chain $ETH_GENESIS_PATH && \
reth node \
    --chain $ETH_GENESIS_PATH \
    --http \
    --http.addr '0.0.0.0' \
    --http.api admin,debug,eth,net,trace,txpool,web3,rpc,reth,ots \
    --authrpc.addr '0.0.0.0' \
    --authrpc.jwtsecret $JWT_PATH \
    --datadir $ETH_DATA_DIR \
	--trusted-peers $ENODE_URLS
    --dev
