# Build stage
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/admin ./cmd/admin

# Runtime stage: tiny image, no shell, runs as non-root
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /out/admin /app/
EXPOSE 8080
ENTRYPOINT ["/app/api"]
