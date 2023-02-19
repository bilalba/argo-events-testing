FROM golang:1.19-alpine

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY pkg ./pkg
COPY main.go ./

ENV GOOS=linux
ENV GOARCH=amd64

RUN go build
RUN mv argo-events-testing /bin

ENTRYPOINT [ "argo-events-testing" ]
