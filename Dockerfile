FROM golang:alpine AS build-env

# Set up dependencies
ENV PACKAGES git build-base

# Set working directory for the build
WORKDIR /go/src/github.com/aerius-labs/scalerize

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
RUN apk add --update ca-certificates jq bash curl
WORKDIR /

# Copy over binaries from the build-env
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/build/scalerized /usr/bin/scalerized
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/example-testnet example-testnet
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/start-testnet-node.sh start-testnet-node.sh
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/testing/files/jwt.hex jwt.hex
COPY --from=build-env /go/src/github.com/aerius-labs/scalerize/scripts/init.sh ./init.sh

# Run scalerized by default
CMD ["scalerized"]