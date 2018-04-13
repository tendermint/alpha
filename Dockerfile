FROM golang:1.10-alpine

RUN apk add --no-cache git

WORKDIR /go/src/alpha
COPY alpha.go .

RUN go get -d -v ./...

# hack
RUN cd /go/src/github.com/tendermint/tendermint && git checkout develop && cd -
RUN cd /go/src/github.com/tendermint/tmlibs && git checkout develop && cd -

RUN go install -v ./...

EXPOSE 8080

CMD ["alpha"]
