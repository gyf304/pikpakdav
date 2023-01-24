FROM golang:1.19 AS builder

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 go build -o pikpakdav

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/pikpakdav /pikpakdav
ENTRYPOINT ["/pikpakdav"]
