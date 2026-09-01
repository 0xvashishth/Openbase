# Build stage
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/openbase-server ./cmd/server

# Runtime stage
FROM alpine:3.20
RUN adduser -D -u 10001 openbase
USER openbase
COPY --from=build /out/openbase-server /usr/local/bin/openbase-server
EXPOSE 8080
ENTRYPOINT ["openbase-server"]