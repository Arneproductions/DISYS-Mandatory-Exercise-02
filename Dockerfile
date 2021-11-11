# syntax=docker/dockerfile:1

FROM golang:1.17-alpine

WORKDIR /app

ENV CLUSTER_HOST="127.0.0.1"

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . ./

RUN go build -o /out ./main.go

EXPOSE 5001 8080

CMD [ "sh", "-c", "/out --clusterAddress ${CLUSTER_HOST}" ]