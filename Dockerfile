FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o stackbill-deployer .

FROM alpine:3.19

RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /app/stackbill-deployer .
COPY --from=builder /app/web ./web
COPY --from=builder /app/scripts ./scripts

EXPOSE 8080

CMD ["./stackbill-deployer"]
