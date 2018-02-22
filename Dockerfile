FROM golang:1.10-alpine3.7

RUN apk -U upgrade && apk add --no-cache -U git


RUN mkdir -p $GOPATH/src/github.com/brkt/metavisor-cli
RUN mkdir /app
ADD . $GOPATH/src/github.com/brkt/metavisor-cli/
WORKDIR $GOPATH/src/github.com/brkt/metavisor-cli

RUN go get github.com/golang/dep/...

RUN dep ensure -vendor-only

ARG GOOS=linux

RUN GOOS=$GOOS GOARCH=amd64 go build -o /app/metavisor cmd/metavisor.go
ENTRYPOINT [ "/app/metavisor" ]