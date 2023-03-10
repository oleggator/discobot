FROM golang:1.19-alpine3.17 as build

WORKDIR /opt/discobot

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /usr/bin/discobot discobot


FROM alpine:3.17

LABEL org.opencontainers.image.source=https://github.com/oleggator/discobot
LABEL org.opencontainers.image.description="discobot"
LABEL org.opencontainers.image.licenses=MIT

COPY --from=build /usr/bin/discobot /usr/bin/discobot
CMD ["/usr/bin/discobot"]
