FROM golang:1 as builder

COPY . /app
WORKDIR /app
RUN make build

FROM chianetwork/chia-docker:latest

ENV service="node"
COPY docker-start.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-start.sh
COPY --from=builder /app/bin/block-metrics /block-metrics
