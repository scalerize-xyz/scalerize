FROM ghcr.io/paradigmxyz/reth:latest

RUN apt-get update && apt-get install -y \
    curl \
    jq \
    iputils-ping

WORKDIR /

COPY ./testing/files ./testing/files
COPY ./start-testnet-reth-bootnode.sh ./start-testnet-reth-bootnode.sh
COPY ./start-testnet-reth-miner-node.sh ./start-testnet-reth-miner-node.sh

RUN chmod +x ./start-testnet-reth-bootnode.sh
RUN chmod +x ./start-testnet-reth-miner-node.sh
