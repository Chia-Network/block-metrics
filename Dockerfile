FROM golang:1 as builder

COPY . /app
WORKDIR /app
RUN make build

FROM alpine:3.18.3

COPY --from=builder /app/bin/block-metrics /block-metrics
CMD ["/block-metrics", "serve"]
