FROM alpine:3.4

COPY . /go/src/github.com/bobrik/zoidbergtcp

RUN apk --update add go && \
    GOPATH=/go go install -v github.com/bobrik/zoidbergtcp/... && \
    apk del go

ENTRYPOINT ["/go/bin/zoidberg-tcp"]
