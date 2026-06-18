FROM golang:1.26-bookworm AS builder
WORKDIR /app
COPY . .
RUN go build -o daemon .

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=builder /app/daemon .
COPY database/migrations ./database/migrations
ENTRYPOINT ["./daemon"]
