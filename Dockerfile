FROM alpine:3.3

COPY . /go/src/github.com/bobrik/zoidbergtcp

RUN apk --update add go && \
    export GOPATH=/go GO15VENDOREXPERIMENT=1 && \
    go get github.com/bobrik/zoidbergtcp/... && \
    apk del go

ENTRYPOINT ["/go/bin/zoidberg-tcp"]
