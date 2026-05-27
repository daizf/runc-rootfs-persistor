FROM golang:1.21-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /wrapper ./cmd/wrapper/

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /wrapper /wrapper

ENTRYPOINT ["/wrapper"]
