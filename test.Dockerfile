FROM --platform=linux/amd64 bornpsych/reth AS reth
WORKDIR /reth
COPY . .

FROM golang:alpine AS build-env

# Set up dependencies
ENV PACKAGES git build-base

# Set working directory for the build
WORKDIR /go/src/github.com/aerius-labs/scalerize
COPY --from=reth /usr/local/bin/reth /usr/local/bin/reth
COPY --from=reth start-testnet-reth-bootnode.sh start-testnet-reth-bootnode.sh
COPY --from=reth start-testnet-reth-miner-node.sh start-testnet-reth-miner-node.sh

# Install dependencies
RUN apk add --update $PACKAGES
RUN apk add linux-headers

# Add source files
COPY . .

# Make the binary
RUN make build
RUN make localtestnet-example-config
RUN echo "completed testnet config"

# Final image
FROM alpine:3.17.3

# Install ca-certificates
RUN apk add --update ca-certificates jq bash curl file
WORKDIR /

# Copy over binaries from the build-env
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/build/scalerized /usr/bin/scalerized
COPY --from=build-env /usr/local/bin/reth /usr/local/bin/reth
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/example-testnet example-testnet
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/start-testnet-node.sh start-testnet-node.sh
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/start-testnet-reth-bootnode.sh start-testnet-reth-bootnode.sh
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/start-testnet-reth-miner-node.sh start-testnet-reth-miner-node.sh
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/scalerize_and_reth.sh scalerize_and_reth.sh
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/testing/files ./testing/files
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/scripts/init.sh init.sh

# Run scalerized by default
# CMD ["scalerized"]