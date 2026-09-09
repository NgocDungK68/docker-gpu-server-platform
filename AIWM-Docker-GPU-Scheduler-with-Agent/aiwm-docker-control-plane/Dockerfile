# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/aiwm-server ./cmd/aiwm-server
RUN mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/aiwm-server /aiwm-server
COPY --from=builder --chown=65532:65532 /data /data
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/aiwm-server"]
