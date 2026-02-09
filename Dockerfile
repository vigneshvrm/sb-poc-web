FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o stackbill-deployer .

FROM alpine:3.19

RUN apk add --no-cache \
    ca-certificates \
    python3 \
    py3-pip \
    sshpass \
    openssh-client \
    && pip3 install --no-cache-dir --break-system-packages ansible-core \
    && addgroup -S appgroup \
    && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/stackbill-deployer .
COPY --from=builder /app/web ./web
COPY --from=builder /app/ansible ./ansible

RUN mkdir -p logs && chown -R appuser:appgroup /app

USER appuser

EXPOSE 9876

CMD ["./stackbill-deployer"]
