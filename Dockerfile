FROM golang:1.20 as go
FROM gcr.io/distroless/base-debian10 as run

FROM go as build
WORKDIR /go/src/github.com/sakajunquality/flow

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /go/bin/server 

FROM run
COPY --from=build /go/bin/server /usr/local/bin/server
CMD ["server"]
