FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY services/api/go.mod services/api/go.sum ./services/api/
WORKDIR /src/services/api
RUN go mod download

WORKDIR /src
COPY services/api ./services/api
COPY db/migrations ./db/migrations

WORKDIR /src/services/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/slai-api ./cmd/slai-api

FROM alpine:3.22

RUN adduser -D -H slai
WORKDIR /app

COPY --from=builder /out/slai-api /app/slai-api
COPY --from=builder /src/db/migrations /app/db/migrations

USER slai
EXPOSE 8080

CMD ["/app/slai-api", "serve"]
