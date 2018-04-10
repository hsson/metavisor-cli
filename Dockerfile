#    Copyright 2018 Immutable Systems, Inc.
# 
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
# 
#        http://www.apache.org/licenses/LICENSE-2.0
# 
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.

FROM golang:1.10-alpine3.7

RUN apk -U upgrade && apk add --no-cache -U git


RUN mkdir -p $GOPATH/src/github.com/immutable/metavisor-cli
RUN mkdir /app
ADD . $GOPATH/src/github.com/immutable/metavisor-cli/
WORKDIR $GOPATH/src/github.com/immutable/metavisor-cli

RUN go get github.com/golang/dep/...

RUN dep ensure -vendor-only

ARG GOOS=linux

RUN GOOS=$GOOS GOARCH=amd64 go build -o /app/metavisor cmd/metavisor.go
ENTRYPOINT [ "/app/metavisor" ]