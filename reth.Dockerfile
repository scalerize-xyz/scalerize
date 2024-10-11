FROM ghcr.io/paradigmxyz/reth:latest
RUN apt-get update && apt-get install -y curl jq
WORKDIR /

COPY ./testing/files ./testing/files
COPY ./start-testnet-reth.sh ./start-testnet-reth.sh
RUN chmod +x ./start-testnet-reth.sh