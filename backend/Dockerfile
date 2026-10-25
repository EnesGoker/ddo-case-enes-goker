# Build context: repo kökü (migrasyonlar db/migrations altında). go.mod ile aynı Go sürümü.
FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/simulator ./cmd/simulator

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /app/api
COPY --from=build /out/simulator /app/simulator
COPY db/migrations /app/migrations
ENV MIGRATIONS_DIR=/app/migrations \
    HTTP_ADDR=:8080
USER nonroot:nonroot
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --start-period=10s --retries=12 CMD ["/app/api", "-healthcheck"]
ENTRYPOINT ["/app/api"]
