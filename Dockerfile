FROM golang:1.11.4-alpine AS builder

RUN apk update && apk add --no-cache git ca-certificates

RUN adduser -D -g '' color

WORKDIR /tmp/color/

COPY  ./go.mod ./go.sum ./vendor ./
RUN go mod vendor

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64  go build \
    -ldflags="-X main.version=$(git describe --tags --always --dirty --long 2>/dev/null || dev.$(date -u +%Y%m%d.%H%M%S))" \
    -mod=vendor -o /tmp/color/color

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd

USER color

ENV PORT 8000
EXPOSE 8000

ENV ADMIN_PORT 9000
EXPOSE 9000

COPY --from=builder /tmp/color/color /go/bin/color

ENTRYPOINT ["/go/bin/color"]
