# ABOUTME: multi-stage build for the daylight CLI binary
# ABOUTME: build stage compiles the Go binary; final stage is minimal alpine with ca-certificates
FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o daylight ./cmd/daylight

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /app/daylight /usr/local/bin/daylight
CMD ["daylight"]
