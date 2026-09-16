<div align="center">
  <p>
    <img src="./Assets/cover.jpg" alt="StreamX cover preview" width="320" />
  </p>
  <h1>StreamX</h1>
  <p>StreamX is a self-hosted music platform that manage your personal library and YouTube Music into one fast, modern experience across Android, web</p>
  <p>Stream, download, and organize your music with ease — from albums and artists to playlists and live jam sessions with friends.</p>
  <p>No ads. No tracking. Just complete control.</p>
  <p>
    <a href="https://github.com/MisfiT2020/StreamXBot/releases">
      <img src="https://img.shields.io/badge/Releases-Android%20Builds-111827?style=for-the-badge&logo=github&logoColor=white" alt="Releases" />
    </a>
    <a href="https://t.me/RaidenEISupport">
      <img src="https://img.shields.io/badge/Support%20Group-Telegram-26A5E4?style=for-the-badge&logo=telegram&logoColor=white" alt="Support Group" />
    </a>
    <a href="#tutorials">
      <img src="https://img.shields.io/badge/Tutorials-Video-FF0000?style=for-the-badge&logo=youtube&logoColor=white" alt="Tutorials" />
    </a>
  </p>
  <p>
    <a href="https://render.com/deploy?repo=https://github.com/MisfiT2020/StreamXBot">
      <img src="https://render.com/images/deploy-to-render-button.svg" alt="Deploy to Render" />
    </a>
  </p>
</div>

## Overview

StreamX is split into three projects that ship one experience:

- `stream`: the root backend, API, Telegram ingit add .
gestion layer, sharing routes, and deployment entrypoint
- `StreamX/`: the Android app
- `StreamXWeb/`: the React web client

The result is a full self-hosted music platform with private infrastructure, cross-device playback, social features, share links, and polished clients on both mobile and web.

## Features

### Playback And Library

- fast playback with queue controls, play next, repeat, shuffle, and rich full-player interactions
- favourites, saved albums, custom playlists, shared playlists, and top-played flows
- album, artist, track, and playlist navigation across both backend and provider-native content
- seamless switching between the StreamX backend library and the YouTube provider inside the app
- YouTube track syncing so songs discovered on YouTube can be brought into StreamX
- offline downloads with download management and remove-download actions
- expanded player views with metadata, next queue, lyrics, and quick actions

### Lyrics, Discovery, And UI

- synced lyrics and dedicated lyrics view
- artist pages, album pages, playlist pages, and search-driven discovery
- direct YouTube search, provider-native YouTube browse flows, and quick provider switching on Android
- customizable UI with polished cards, home sections, dark mode, and AMOLED-style presentation
- polished Compose-based UI with full-screen player, bottom sheets, and modern navigation

### Social And Sharing

- collaborative jam sessions with synchronized playback and queue updates
- jam invites, join flows, cooldown handling, and push notifications
- friends, presence, listening activity, and share-listening controls
- share links for playlists, albums, tracks, and jams

## Screenshots

| 1 | 2 | 3 |
| --- | --- | --- |
| ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/1.jpg) | ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/2.jpg) | ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/3.jpg) |

| 4 | 5 | 6 |
| --- | --- | --- |
| ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/4.jpg) | ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/5.jpg) | ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/6.jpg) |

| 7 | 8 | 9 |
| --- | --- | --- |
| ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/7.jpg) | ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/8.jpg) | ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/9.jpg) |

| 10 | 11 | 12 |
| --- | --- | --- |
| ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/10.jpg) | ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/11.jpg) | ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/12.jpg) |

| 13 | 14 |  |
| --- | --- | --- |
| ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/13.jpg) | ![](https://raw.githubusercontent.com/MisfiT2020/src/main/streamx/app/14.jpg) |  |

## Project Layout

| Project | Role | Stack | Docs |
| --- | --- | --- | --- |
| `stream` | Backend, API, sharing, deployment | Python, FastAPI, MongoDB, Telegram, Firebase Admin | [README.md](./README.md) |
| `StreamX/` | Android app | Kotlin, Jetpack Compose, Media3, Firebase Messaging | [StreamX/README.md](./StreamX/README.md) |
| `StreamXWeb/` | Web app | React, Vite, TypeScript | [StreamXWeb/README.md](./StreamXWeb/README.md) |

## Releases

- Android APK builds are published in the root repo's [Releases](https://github.com/MisfiT2020/StreamXBot/releases)
- `StreamXWeb` is built and served by the backend in normal production deployments
- backend deployment is source-based or Docker-based depending on your environment

## Quick Start

### 1. Start the backend

```bash
python -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt
copy sample_config.py config.py
python -m stream
```

Or with Docker:

```bash
docker compose up --build
```

### 2. Build the Android app

```bash
cd StreamX
.\gradlew.bat :app:assembleDebug
.\gradlew.bat :app:installDebug
```

Before the first build, place `google-services.json` at `StreamX/app/google-services.json`.

### 3. Run the web app locally

```bash
cd StreamXWeb
npm install
npm run dev
```

For production, `StreamXWeb` is usually not deployed separately. The backend Dockerfile already builds and serves it.

## Backend Overview

This root project powers:

- auth, playlists, favourites, albums, tracks, friends, jams, and notifications
- Telegram ingestion and admin tooling
- media streaming endpoints
- playlist, album, track, and jam share routes
- production serving of the built web app

Main entrypoint:

```bash
python -m stream
```

You can run it in:

- API-only=True mode disables the bot's runtime, which is useful for testing  

## Deployment

### Local Python

Requirements:

- Python 3.11+

Run:

```bash
python -m venv .venv
.venv\Scripts\activate
pip install --upgrade pip
pip install -r requirements.txt
copy sample_config.py config.py
python -m stream
```

### Docker

The root `Dockerfile` already:

1. builds `WebX`
2. installs Python dependencies in a virtual environment
3. copies backend code
4. copies the built web `dist/`
5. starts the app directly via `python3 -m stream`

Run:

```bash
docker compose up --build -d
```

### Render

This repo can be used for other service (free) such as koyeb.

Typical Render flow:

1. create a Docker web service
2. point it at this repo
3. set `CONFIG_GIST` to a raw URL that returns your full `config.py`
4. deploy

One-click shortcut:

- [Deploy to Render](https://render.com/deploy?repo=https://github.com/MisfiT2020/StreamXBot)

Health check:

```text
GET /health
```

## Tutorials

| Topic | Video |
| --- | --- |
| VPS Setup | [YouTube](https://www.youtube.com/watch?v=n36uEef8VrE) |
| Render / Koyeb / Other Platforms | [YouTube](https://www.youtube.com/watch?v=A6kBOLGAbnk) |

## Main Config

Use `sample_config.py` as the template for `config.py`.

### Core values for most full installs

| Key | Purpose |
| --- | --- |
| `BOT_TOKEN` | Telegram bot token |
| `API_ID` | Telegram API ID |
| `API_HASH` | Telegram API hash |
| `MONGO_URI` | MongoDB connection string |
| `DATABASE_NAME` | MongoDB database name |
| `OWNER_ID` | Telegram owner ID |
| `SECRET_KEY` | Session signing and Firebase credential decoding |
| `CHANNEL_ID` | Main Telegram source channel |
| `FILTER_MODE` | `0` indexes only `CHANNEL_ID`; `1` indexes media from any chat, group, or forum topic the bot can read |

### Common optional values

| Key | Purpose |
| --- | --- |
| `FIREBASE_CREDENTIALS` | Encoded Firebase Admin service account |
| `CORS_ORIGINS` | Allowed frontend origins |
| `COOKIE_SECURE` | Secure cookie flag for HTTPS |
| `COOKIE_SAMESITE` | Cross-site cookie policy |
| `SPOTIFY_CLIENT_ID` / `SPOTIFY_CLIENT_SECRET` | Spotify metadata integration |
| `SESSION_STRING` | Optional userbot session |
| `SOURCE_CHANNEL_IDS` | Additional Telegram source channels |
| `DUMP_CHANNEL_ID` | Optional dump channel |
| `LRCLIB` / `MUSIXMATCH` | Lyrics provider toggles |
| `MULTI_CLIENTS`, `MULTI_CLIENTS_1...` | Extra Telegram clients or tokens |

### API-only mode

If you only want the API:

```python
ONLY_API = True
```

In that mode the important requirements are:

- `MONGO_URI`
- `DATABASE_NAME`
- `SECRET_KEY`

## Firebase Setup For The Backend

The backend uses Firebase Admin to send push notifications. This is different from the Android device token.

### What you need

You need a Firebase service account JSON from:

1. Firebase Console
2. Project settings
3. Service accounts
4. Generate new private key

Save that file as `service_account.json` in the root of this repo.

### How to use `encode_firebase.py`

This repo includes `encode_firebase.py` so you can store an encoded value in `config.py` instead of raw JSON.

Steps:

1. put the downloaded file at `service_account.json`
2. make sure `SECRET_KEY` is already set in `config.py`
3. run:

```bash
python encode_firebase.py
```

4. copy the printed output into:

```python
FIREBASE_CREDENTIALS = "..."
```

### Alternate Firebase env option

The backend also supports:

```text
FIREBASE_CRED_B64
```

That should be plain base64 of the raw Firebase service account JSON.

### Firebase value checklist

| Item | Used by | Where you get it |
| --- | --- | --- |
| `service_account.json` / `FIREBASE_CREDENTIALS` / `FIREBASE_CRED_B64` | Backend push sending | Firebase Console -> Project settings -> Service accounts |
| `google-services.json` | Android app build setup | Firebase Console -> Project settings -> Your apps -> Android |
| FCM device token | Individual Android device | Generated automatically by the app |

Do not paste the device FCM token into backend config. The Android client obtains it automatically and registers it after login.

## YouTube Cookies

Backend-side YouTube extraction looks for:

```text
cookies/yt.txt
```

Use it when you need stronger `yt-dlp` access for restricted or rate-limited content.

### How to prepare it

1. log in to YouTube in a desktop browser
2. export cookies in Netscape format
3. save the file as `cookies/yt.txt`

Notes:

- keep the file private and never commit it
- you can upload the cookies directly via telegram bot using /sudo > cookies > save cookies as yt.txt and upload.

## Support

- Support group: [t.me/RaidenEISupport](https://t.me/RaidenEISupport)

## Thanks

- Metrolist for innertube & reference

## Project Docs

- Root backend and deployment: [README.md](./README.md)
- Android app: [StreamX/README.md](./StreamX/README.md)
- Web app: [StreamXWeb/README.md](./StreamXWeb/README.md)
