FROM ghcr.io/paradigmxyz/reth:latest
RUN apt-get update && apt-get install -y curl jq
WORKDIR /

COPY ./testing/files ./testing/files
COPY ./start-testnet-reth.sh ./start-testnet-reth.sh
COPY ./add-reth-peers.sh ./add-reth-peers.sh
RUN chmod +x ./start-testnet-reth.sh
RUN chmod +x ./add-reth-peers.sh