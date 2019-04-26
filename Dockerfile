FROM alpine

RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

COPY ./emojify-traffic .

ENTRYPOINT ./emojify-traffic
