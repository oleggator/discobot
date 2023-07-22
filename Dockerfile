FROM --platform=$BUILDPLATFORM golang:1.20-alpine3.17 as build

WORKDIR /opt/discobot

COPY go.mod go.sum ./
RUN go mod download

ARG TARGETOS
ARG TARGETARCH

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /usr/bin/discobot discobot/cmd/discobot


FROM alpine:3.17

LABEL org.opencontainers.image.source=https://github.com/oleggator/discobot
LABEL org.opencontainers.image.description="discobot"
LABEL org.opencontainers.image.licenses=MIT

RUN apk add yt-dlp ffmpeg

COPY --from=build /usr/bin/discobot /usr/bin/discobot
CMD ["/usr/bin/discobot"]
