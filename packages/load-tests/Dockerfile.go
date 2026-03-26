FROM golang:1.26-alpine AS build

COPY go/memory-hog/ /src/

WORKDIR /src

RUN go build -o /bin/job .

FROM alpine:3.21

COPY --from=build /bin/job /usr/local/bin/job

ENTRYPOINT ["job"]
