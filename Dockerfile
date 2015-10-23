FROM alpine:3.2

COPY . /go/src/github.com/bobrik/zoidbergtcp

RUN apk --update add go && \
    export GOPATH=/go:/go/src/github.com/bobrik/zoidbergtcp/Godeps/_workspace && \
    go get github.com/bobrik/zoidbergtcp/... && \
    apk del go
