FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o worker ./cmd/worker

FROM alpine:latest AS server
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/server .
COPY --from=builder /app/worker .
COPY --from=builder /app/web ./web
EXPOSE 8080
# nobody в Alpine. Совпадает с runAsUser в k8s/deployment.yaml, порт 8080 ему доступен.
USER 65534:65534
CMD ["./server"]
