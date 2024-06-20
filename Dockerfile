FROM golang:1.22 AS builder
COPY . /src
WORKDIR /src
RUN CGO_ENABLED=0 go build -o containervm github.com/cox96de/containervm/cmd/containervm
FROM debian
RUN apt update && apt install -y qemu-system
COPY --from=builder /src/containervm /opt
ENTRYPOINT ["/opt/containervm"]
