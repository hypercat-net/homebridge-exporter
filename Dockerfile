# Build stage
FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /homebridge-exporter ./cmd/homebridge-exporter

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /homebridge-exporter /homebridge-exporter

EXPOSE 9090
USER nonroot:nonroot
ENTRYPOINT ["/homebridge-exporter"]
