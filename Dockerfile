# Build static binaries for all services.
FROM golang:1.25-alpine AS build
RUN apk add --no-cache ca-certificates git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api-gateway ./cmd/api-gateway \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bet-service ./cmd/bet-service \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/outbox-relay ./cmd/outbox-relay \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/wallet-service ./cmd/wallet-service \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bet-worker ./cmd/bet-worker \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/risk-service ./cmd/risk-service \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/notification-service ./cmd/notification-service \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/settlement-service ./cmd/settlement-service \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/odds-service ./cmd/odds-service \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.19
RUN apk add --no-cache ca-certificates \
 && adduser -D -H -s /sbin/nologin appuser
USER appuser
WORKDIR /app
COPY --from=build /out/api-gateway /app/api-gateway
COPY --from=build /out/bet-service /app/bet-service
COPY --from=build /out/outbox-relay /app/outbox-relay
COPY --from=build /out/wallet-service /app/wallet-service
COPY --from=build /out/bet-worker /app/bet-worker
COPY --from=build /out/risk-service /app/risk-service
COPY --from=build /out/notification-service /app/notification-service
COPY --from=build /out/settlement-service /app/settlement-service
COPY --from=build /out/odds-service /app/odds-service
COPY --from=build /out/migrate /app/migrate
EXPOSE 8080
CMD ["/app/api-gateway"]
