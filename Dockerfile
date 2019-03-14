FROM scratch

COPY ./emojify-traffic .

ENTRYPOINT ./emojify-traffic
