# syntax=docker/dockerfile:1

FROM golang:1.24

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY static/ static/
COPY templates templates/

RUN go build -o /file-cloud

EXPOSE 8080

CMD ["/file-cloud"]
