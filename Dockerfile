FROM golang:1.19 as builder

WORKDIR /build
COPY .. ./

# ldflags -w and -s result in smaller binary.
# -w Omits the DWARF symbol table.
# -s Omits the symbol table and debug information.
# RUN CGO_ENABLED=0 go build -mod=mod -a -ldflags "-w -s" -o smtpbox ./cmd/smtpbox/main.go
RUN CGO_ENABLED=0 go build -mod=mod -o smtpbox ./cmd/smtpbox/main.go

FROM scratch
COPY --from=builder /build/smtpbox /smtpbox

ADD ../build/ca-certificates.crt /etc/ssl/certs/
ADD ../build/zoneinfo.zip /zoneinfo.zip

ENV ZONEINFO "/zoneinfo.zip"

CMD ["/smtpbox"]
