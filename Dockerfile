FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /kb-server ./cmd/server

# Create data directories owned by nonroot (UID 65534)
RUN mkdir -p /data/knowledge/bundle /data/knowledge/.versions && \
    chown -R 65534:65534 /data

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /kb-server /kb-server
COPY --from=build --chown=nonroot:nonroot /data /data

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/kb-server"]
