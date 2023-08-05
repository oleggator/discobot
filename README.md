# Discobot - Minimalistic Discord Music Bot
[![Build and deploy](https://github.com/oleggator/discobot/actions/workflows/fly.yml/badge.svg)](https://github.com/oleggator/discobot/actions/workflows/fly.yml)

Supports all services supported by [yt-dlp](https://github.com/yt-dlp/yt-dlp), including YouTube, SoundCloud, Twitch, TuneIn, etc.

## How to run

### With Docker

#### Pull from repository
```shell
docker run -it --rm -e TOKEN=DISCORD_BOT_TOKEN ghcr.io/oleggator/discobot:latest
```

#### Or build by yourself
Docker image is buildable for the next platforms: x86_64, x86, aarch64, armhf, s390x, armv7, ppc64le
```shell
docker build -t discobot .
```

### Without Docker

## Prerequisites
- Go 1.20
- yt-dlp
- ffmpeg

```shell
git clone https://github.com/oleggator/discobot.git
cd discobot
go build discobot
TOKEN=DISCORD_BOT_TOKEN ./discobot
```
