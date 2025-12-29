FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o app .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /build/app /app/app
COPY --from=builder /build/views /app/views
EXPOSE 5000
CMD ["/app/app"]
