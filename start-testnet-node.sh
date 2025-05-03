#!/bin/sh

# Copy the folder based on ID environment variable
cp -r ./example-testnet/node$ID/scalerized ~/.scalerized
CONFIG_PATH=~/.scalerized/config/config.toml

PERSISTENT_PEERS=$(grep "^persistent_peers =" "$CONFIG_PATH" | cut -d '"' -f2)

if [ -z "$PERSISTENT_PEERS" ]; then
    echo "No persistent peers found in config.toml"
    exit 1
fi

echo "Found persistent peers: $PERSISTENT_PEERS"


NODE_IDS=$(echo "$PERSISTENT_PEERS" | sed 's/@[^,]*//g')

echo "Extracted node IDs: $NODE_IDS"

if grep -q "^unconditional_peer_ids =" "$CONFIG_PATH"; then
    sed -i "s/^unconditional_peer_ids = .*/unconditional_peer_ids = \"$NODE_IDS\"/" "$CONFIG_PATH"
else
    sed -i "/\[p2p\]/a unconditional_peer_ids = \"$NODE_IDS\"" "$CONFIG_PATH"
fi

echo "Updated unconditional_peer_ids in $CONFIG_PATH"

# Start the scalerized node
echo "starting scalerize node $ID in background ..."
scalerized start --execution-client-type=evm --engine-api-url=$ENGINE_API --eth-rpc-url=$RPC_API
tail -f /dev/null