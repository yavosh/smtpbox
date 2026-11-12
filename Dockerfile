FROM --platform=$BUILDPLATFORM golang:1.27.4 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build
COPY . ./

# ldflags -w and -s result in smaller binary.
# -w Omits the DWARF symbol table.
# -s Omits the symbol table and debug information.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=vendor -a -ldflags "-w -s" -o smtpbox ./cmd/smtpbox

FROM scratch
LABEL org.opencontainers.image.source="https://github.com/yavosh/smtpbox"
COPY --from=builder /build/smtpbox /smtpbox

ADD build/ca-certificates.crt /etc/ssl/certs/
ADD build/zoneinfo.zip /zoneinfo.zip

ENV ZONEINFO=/zoneinfo.zip
ENV LISTEN_HOST=0.0.0.0

EXPOSE 8080/tcp 1025/tcp 1053/udp
USER 65532:65532
ENTRYPOINT ["/smtpbox"]
