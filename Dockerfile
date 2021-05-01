FROM golang:1.16 as builder
WORKDIR /go/app
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o ./bin -ldflags="-extldflags=-static" ./cmd/...

FROM gcr.io/distroless/static
LABEL org.opencontainers.image.source=https://github.com/patrick246/k8s-outdated-image-exporter
COPY --from=builder /go/app/bin/outdated-image-exporter /outdated-image-exporter
ENTRYPOINT ["/outdated-image-exporter"]
