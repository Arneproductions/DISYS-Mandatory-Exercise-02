# syntax=docker/dockerfile:1

FROM golang:1.17-alpine

WORKDIR /app

ENV CLIENTS=""

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . ./

RUN go build -o /out ./main.go

EXPOSE 5001 8080 80 1 

CMD [ "/out" ]