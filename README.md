<h1 align="center">StreamGO</h1>

<p align="center">A Go server for streaming music from Telegram. It stores track details in MongoDB and serves the WebX music player.</p>

<p align="center"><a href="https://github.com/StreamXProject/StreamX">StreamX for Android</a></p>

StreamXBot's Go version, called StreamGO. The previous Python source is on [`legacy/python`](https://github.com/StreamXProject/StreamXBot/tree/legacy/python).

- Search tracks, artists, and albums.
- Save playlists and favourites.
- View lyrics, listening history, and recaps.
- Manage music sources and user access through a Telegram bot.

## Run locally

You need Go 1.25 or newer, MongoDB, FFmpeg, and MediaInfo.

```sh
cp .env.example .env
```

Edit `.env` and set:

| Variable | Value |
| --- | --- |
| `MONGO_URI` | Your MongoDB connection string |
| `API_ID`, `API_HASH` | Your Telegram API credentials |
| `BOT_TOKEN` | Your Telegram bot token |
| `CHANNEL_ID` | The channel or group to read music from |
| `OWNER_ID` | Your Telegram user ID |
| `SECRET_KEY` | A long, random secret for login tokens |

Add the bot to your music channel or group. From the project folder, install and run:

```sh
go install ./cmd/server
server
```

Make sure your Go bin folder (`go env GOBIN`, or `$(go env GOPATH)/bin` if unset) is in your `PATH`. Run `server` from the folder containing `.env`.

The server runs at [localhost:8000](http://localhost:8000). Send audio to the configured channel or group to add tracks. Send `/help` to the bot for commands.

To use the web player, place a WebX build in `dist/` with `dist/index.html` present. Cloudflare R2 is optional for storing artwork; its settings are in `.env.example`.

## Docker

Use the same `.env`, with a `MONGO_URI` the container can reach. Compose runs only StreamGO; MongoDB must run separately.

```sh
docker compose up -d --build
```

The image builds and includes WebX. Set environment variables in your hosting service when deploying on Render or Heroku.

## API

- [API docs](http://localhost:8000/docs)
- [OpenAPI spec](http://localhost:8000/openapi.json)
- [Service status](http://localhost:8000/health)
