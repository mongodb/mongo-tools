
FROM golang:1.27.3

WORKDIR /go/src/github.com/samber/lo

COPY Makefile go.* ./

RUN make tools
