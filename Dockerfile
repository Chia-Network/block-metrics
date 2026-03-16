FROM golang:1 AS builder

COPY . /app
WORKDIR /app
RUN make build

FROM gcr.io/distroless/static-debian13:latest

COPY --from=builder /app/bin/block-metrics /block-metrics
USER nonroot:nonroot
CMD ["/block-metrics", "serve"]
