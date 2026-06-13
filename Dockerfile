FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod ./
# COPY go.sum ./  # uncomment if go.sum exists
RUN go mod download

COPY . .
RUN go build -o transparent cmd/transparent/main.go

FROM alpine:latest

RUN apk add --no-cache git openssh-client tzdata docker-cli

WORKDIR /app
COPY --from=builder /app/transparent .

# Ensure git is configured with a dummy user if not mounted
RUN git config --global user.email "transparent@local" && \
    git config --global user.name "Transparent Daemon"

CMD ["./transparent", "-repo", "/data"]
