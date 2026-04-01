FROM golang:1.26 AS builder

WORKDIR /workspace

# Download modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /server ./server.go

FROM gcr.io/distroless/cc-debian11

COPY --from=builder /server /server

EXPOSE 8080
ENV PORT=8080

ENTRYPOINT ["/server"]
