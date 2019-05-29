FROM golang:1.12 AS build

COPY . /warscript-bots
WORKDIR /warscript-bots

RUN go build .

FROM alpine:latest

RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2
COPY --from=build /warscript-bots/warscript-bots /warscript-bots

CMD [ "/warscript-bots" ]