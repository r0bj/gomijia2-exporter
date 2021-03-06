FROM golang:1.17.6 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY *.go .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a --ldflags '-w -extldflags "-static"' -tags netgo -installsuffix netgo -o gomijia2-exporter .


FROM alpine:3.15

COPY --from=builder /workspace/gomijia2-exporter /

ENTRYPOINT ["/gomijia2-exporter"]
