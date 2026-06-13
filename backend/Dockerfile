# ---- Build stage ----
FROM golang:1.23 AS builder

WORKDIR /app

# Copy dependency manifests first, download deps.
# This layer is cached unless go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source and build.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# ---- Run stage ----
FROM alpine:3.20

WORKDIR /app

# Copy only the compiled binary from the build stage.
COPY --from=builder /server /server

EXPOSE 8080

CMD ["/server"]