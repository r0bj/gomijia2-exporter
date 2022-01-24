FROM golang:1.17.6 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY *.go .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a --ldflags '-w -extldflags "-static"' -tags netgo -installsuffix netgo -o gomijia2-exporter .


FROM alpine:3.15

COPY --from=builder /workspace/gomijia2-exporter /

# Sleep is a workaround for: can't init hci: no devices available: (hci0: can't up device: interrupted system call)
ENTRYPOINT ["sh", "-c", "sleep 3; /gomijia2-exporter"]
