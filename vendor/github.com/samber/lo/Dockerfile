
FROM golang:1.26.10

WORKDIR /go/src/github.com/samber/lo

COPY Makefile go.* ./

RUN make tools
