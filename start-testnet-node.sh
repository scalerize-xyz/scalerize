# !/bin/sh

# Copy the folder based on ID environment variable
cp -r ./example-testnet/node$ID/scalerized /root/.scalerized

# Start the scalerized node
echo "starting scalerize node $ID in background ..."
scalerized start

tail -f /dev/null