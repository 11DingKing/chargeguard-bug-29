FROM golang:1.26 AS builder
ENV GOTOOLCHAIN=local
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /chargeguard ./cmd/chargeguard
RUN CGO_ENABLED=0 go build -o /chargeguardctl ./cmd/chargeguardctl

FROM golang:1.26
COPY --from=builder /chargeguard /chargeguard
COPY --from=builder /chargeguardctl /chargeguardctl
EXPOSE 56058
ENTRYPOINT ["/chargeguard"]
