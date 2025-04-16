# !/bin/sh

# Copy the folder based on ID environment variable
cp -r ./example-testnet/node$ID/scalerized /root/.scalerized

# Start the scalerized node
echo "starting scalerize node $ID in background ..."
scalerized start --execution-client-type=evm --engine-api-url=$ENGINE_API --eth-rpc-url=$RPC_API --log_level=debug 
tail -f /dev/null