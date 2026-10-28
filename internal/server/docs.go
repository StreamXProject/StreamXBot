package server

import (
	"net/http"
)

const openAPISpecJSON = `{
  "openapi": "3.0.3",
  "info": {
    "title": "StreamGO API",
    "description": "High-Performance Media Streaming, Discovery, Access Control, and Telegram MiniApp Backend written in Go",
    "version": "1.0.0"
  },
  "servers": [
    {
      "url": "/",
      "description": "Current Server"
    }
  ],
  "components": {
    "securitySchemes": {
      "bearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT / v1.<b64url>.<sig>"
      },
      "guestPassword": {
        "type": "apiKey",
        "name": "X-Guest-Password",
        "in": "header"
      }
    }
  },
  "paths": {
    "/": {
      "get": {
        "tags": ["System"],
        "summary": "Root Service Info",
        "description": "Returns basic service metadata, status, and running version.",
        "responses": { "200": { "description": "Successful Response" } }
      }
    },
    "/health": {
      "get": {
        "tags": ["System"],
        "summary": "Health Check",
        "description": "Returns server uptime, current time, Telegram client state, and real-time MongoDB connectivity status.",
        "responses": { "200": { "description": "Health status response" } }
      }
    },
    "/tracks": {
      "get": {
        "tags": ["Tracks"],
        "summary": "Browse / List Tracks",
        "description": "Paginated track list with sorting, topic filtering, and channel filtering.",
        "parameters": [
          { "name": "page", "in": "query", "schema": { "type": "integer", "default": 1 } },
          { "name": "per_page", "in": "query", "schema": { "type": "integer", "default": 20 } },
          { "name": "sort", "in": "query", "schema": { "type": "string", "enum": ["updated_at", "play_count", "created_at"] } },
          { "name": "topic", "in": "query", "schema": { "type": "string" } },
          { "name": "channel_id", "in": "query", "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "Paginated list of browse items" } }
      }
    },
    "/tracks/{id}": {
      "get": {
        "tags": ["Tracks"],
        "summary": "Get Track by ID",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "200": { "description": "Full track entity" },
          "404": { "description": "Track not found" }
        }
      }
    },
    "/tracks/random": {
      "get": {
        "tags": ["Tracks"],
        "summary": "Get Random Tracks",
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 20 } },
          { "name": "channel_id", "in": "query", "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "Randomized selection of tracks" } }
      }
    },
    "/tracks/shuffle": {
      "get": {
        "tags": ["Tracks"],
        "summary": "Shuffle Library Tracks",
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } },
          { "name": "channel_id", "in": "query", "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "Shuffled track queue" } }
      }
    },
    "/library/shuffle": {
      "get": {
        "tags": ["Tracks"],
        "summary": "Alias for Library Shuffle",
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } }
        ],
        "responses": { "200": { "description": "Shuffled track queue" } }
      }
    },
    "/search": {
      "get": {
        "tags": ["Tracks"],
        "summary": "Search Tracks",
        "description": "Search tracks by title, artist, performer, or album.",
        "parameters": [
          { "name": "q", "in": "query", "required": true, "schema": { "type": "string" } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } }
        ],
        "responses": { "200": { "description": "Search results matching query" } }
      }
    },
    "/topics": {
      "get": {
        "tags": ["Topics"],
        "summary": "List Unique Topics",
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } }
        ],
        "responses": { "200": { "description": "List of distinct topic tags" } }
      }
    },
    "/topics/{name}/tracks": {
      "get": {
        "tags": ["Topics"],
        "summary": "Get Tracks by Topic",
        "parameters": [
          { "name": "name", "in": "path", "required": true, "schema": { "type": "string" } },
          { "name": "page", "in": "query", "schema": { "type": "integer", "default": 1 } },
          { "name": "per_page", "in": "query", "schema": { "type": "integer", "default": 20 } }
        ],
        "responses": { "200": { "description": "Tracks matching topic" } }
      }
    },
    "/stream/{id}": {
      "get": {
        "tags": ["Streaming"],
        "summary": "Stream Audio Chunk / HTTP 206 Partial Content",
        "description": "High-performance streaming directly from Telegram MTProto multi-client connection pool with range requests support.",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } },
          { "name": "Range", "in": "header", "schema": { "type": "string" } }
        ],
        "responses": {
          "200": { "description": "Full file download" },
          "206": { "description": "Partial byte range stream" }
        }
      }
    },
    "/tracks/{id}/download": {
      "get": {
        "tags": ["Streaming"],
        "summary": "Download Track File",
        "description": "Streams file with Content-Disposition attachment header for direct file downloading.",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Audio file download stream" } }
      }
    },
    "/tracks/{id}/warm": {
      "get": {
        "tags": ["Streaming"],
        "summary": "Warm Track Cache",
        "description": "Pre-fetches initial audio chunks into RAM buffer to reduce TTFB on play.",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Warm status" } }
      }
    },
    "/stream/cover/{id}": {
      "get": {
        "tags": ["Streaming"],
        "summary": "Stream Track Album Art / Thumbnail",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Image bytes" } }
      }
    },
    "/cover/{id}": {
      "get": {
        "tags": ["Streaming"],
        "summary": "Track Cover Redirect",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "302": { "description": "Redirect to cover image URL" } }
      }
    },
    "/tracks/{id}/lyrics": {
      "get": {
        "tags": ["Media"],
        "summary": "Track Lyrics",
        "description": "Returns synchronized or plain lyrics for a track in JSON or text/plain.",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } },
          { "name": "format", "in": "query", "schema": { "type": "string", "enum": ["plain", "json"] } }
        ],
        "responses": {
          "200": { "description": "Lyrics content" },
          "404": { "description": "Lyrics not found" }
        }
      }
    },
    "/artists": {
      "get": {
        "tags": ["Artists"],
        "summary": "List Artists",
        "parameters": [
          { "name": "page", "in": "query", "schema": { "type": "integer", "default": 1 } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 20 } },
          { "name": "q", "in": "query", "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "List of artists" } }
      }
    },
    "/artists/{id}": {
      "get": {
        "tags": ["Artists"],
        "summary": "Get Artist Details",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Artist metadata and biography" } }
      }
    },
    "/artists/{id}/tracks": {
      "get": {
        "tags": ["Artists"],
        "summary": "Get Artist Top / Popular Tracks",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Tracks by artist" } }
      }
    },
    "/artists/{id}/albums": {
      "get": {
        "tags": ["Artists"],
        "summary": "Get Albums by Artist",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Discography by artist" } }
      }
    },
    "/albums": {
      "get": {
        "tags": ["Albums"],
        "summary": "List Albums",
        "parameters": [
          { "name": "page", "in": "query", "schema": { "type": "integer", "default": 1 } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 20 } },
          { "name": "q", "in": "query", "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "List of albums" } }
      }
    },
    "/albums/{id}": {
      "get": {
        "tags": ["Albums"],
        "summary": "Get Album Details",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Album metadata and tracks" } }
      }
    },
    "/albums/{id}/tracks": {
      "get": {
        "tags": ["Albums"],
        "summary": "Get Album Tracks",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Ordered tracks in album" } }
      }
    },
    "/favourites": {
      "get": {
        "tags": ["Favourites"],
        "summary": "List User Favourite Tracks",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "page", "in": "query", "schema": { "type": "integer", "default": 1 } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 20 } }
        ],
        "responses": { "200": { "description": "User favourite tracks" } }
      },
      "post": {
        "tags": ["Favourites"],
        "summary": "Add Track to Favourites",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["track_id"],
                "properties": { "track_id": { "type": "string" } }
              }
            }
          }
        },
        "responses": { "200": { "description": "Favourite saved" } }
      }
    },
    "/favourites/{id}": {
      "delete": {
        "tags": ["Favourites"],
        "summary": "Remove Track from Favourites",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Removed from favourites" } }
      }
    },
    "/favourites/ids": {
      "get": {
        "tags": ["Favourites"],
        "summary": "Get All Favourite Track IDs",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "responses": { "200": { "description": "List of liked track IDs" } }
      }
    },
    "/favourites/artists": {
      "get": {
        "tags": ["Favourites"],
        "summary": "List Followed Artists",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "responses": { "200": { "description": "List of followed artists" } }
      },
      "post": {
        "tags": ["Favourites"],
        "summary": "Follow Artist",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["artist_id"],
                "properties": { "artist_id": { "type": "string" } }
              }
            }
          }
        },
        "responses": { "200": { "description": "Artist followed" } }
      }
    },
    "/favourites/artists/{id}": {
      "delete": {
        "tags": ["Favourites"],
        "summary": "Unfollow Artist",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Artist unfollowed" } }
      }
    },
    "/favourites/albums": {
      "get": {
        "tags": ["Favourites"],
        "summary": "List Saved Albums",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "responses": { "200": { "description": "List of saved albums" } }
      },
      "post": {
        "tags": ["Favourites"],
        "summary": "Save Album to Library",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["album_id"],
                "properties": { "album_id": { "type": "string" } }
              }
            }
          }
        },
        "responses": { "200": { "description": "Album saved" } }
      }
    },
    "/history": {
      "get": {
        "tags": ["Playback History & Telemetry"],
        "summary": "Get Playback History (/history)",
        "description": "Returns user playback event history and recently played tracks with timestamps.",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } }
        ],
        "responses": { "200": { "description": "User playback history" } }
      }
    },
    "/me/history": {
      "get": {
        "tags": ["Playback History & Telemetry"],
        "summary": "Get My History (/me/history)",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } }
        ],
        "responses": { "200": { "description": "Recent listening history" } }
      }
    },
    "/me/top-played": {
      "get": {
        "tags": ["Playback History & Telemetry"],
        "summary": "Get User Top Played Tracks (/me/top-played)",
        "description": "Aggregates personal play counts and returns the most played tracks for the current user.",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } }
        ],
        "responses": { "200": { "description": "Top played tracks" } }
      }
    },
    "/me/listening-events": {
      "post": {
        "tags": ["Playback History & Telemetry"],
        "summary": "Log Playback Events / Scrobble (/me/listening-events)",
        "description": "Records play, pause, seek, completion, and scrobble telemetry events for user and global metrics.",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["events"],
                "properties": {
                  "events": {
                    "type": "array",
                    "items": {
                      "type": "object",
                      "required": ["track_id", "event_type"],
                      "properties": {
                        "track_id": { "type": "string" },
                        "event_type": { "type": "string", "enum": ["play", "scrobble", "pause", "resume", "seek", "complete"] },
                        "position_sec": { "type": "number" },
                        "duration_sec": { "type": "integer" },
                        "timestamp": { "type": "number" }
                      }
                    }
                  }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Events recorded" } }
      }
    },
    "/listening-events": {
      "post": {
        "tags": ["Playback History & Telemetry"],
        "summary": "Alias for Listening Events (/listening-events)",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "responses": { "200": { "description": "Events recorded" } }
      }
    },
    "/playlists": {
      "get": {
        "tags": ["Playlists"],
        "summary": "List User Playlists",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "responses": { "200": { "description": "List of user playlists" } }
      },
      "post": {
        "tags": ["Playlists"],
        "summary": "Create Custom Playlist",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["title"],
                "properties": {
                  "title": { "type": "string" },
                  "description": { "type": "string" },
                  "cover_url": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Playlist created" } }
      }
    },
    "/playlists/{id}": {
      "get": {
        "tags": ["Playlists"],
        "summary": "Get Playlist by ID",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Playlist detail" } }
      },
      "patch": {
        "tags": ["Playlists"],
        "summary": "Update Playlist",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Playlist updated" } }
      },
      "delete": {
        "tags": ["Playlists"],
        "summary": "Delete Playlist",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Playlist deleted" } }
      }
    },
    "/playlists/{id}/tracks": {
      "get": {
        "tags": ["Playlists"],
        "summary": "Get Playlist Tracks",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Tracks in playlist" } }
      },
      "post": {
        "tags": ["Playlists"],
        "summary": "Add Tracks to Playlist",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Tracks added" } }
      }
    },
    "/playlists/{id}/reorder": {
      "post": {
        "tags": ["Playlists"],
        "summary": "Reorder Playlist Tracks",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Playlist reordered" } }
      }
    },
    "/playlists/available": {
      "get": {
        "tags": ["Daily Dynamic Playlists"],
        "summary": "List Available Dynamic Playlists",
        "description": "Returns metadata for all available daily dynamic mixes (Daily Mix 1–6, Discover Weekly, Rediscover, Late Night, Rising, Surprise Me).",
        "responses": { "200": { "description": "List of available mixes" } }
      }
    },
    "/daily-playlist/{key}": {
      "get": {
        "tags": ["Daily Dynamic Playlists"],
        "summary": "Get Daily Dynamic Playlist (/daily-playlist/{key})",
        "description": "Generates or retrieves cached daily dynamic mix generated from user listening habits and library genres.",
        "parameters": [
          { "name": "key", "in": "path", "required": true, "schema": { "type": "string", "example": "daily_mix_1" } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 75 } },
          { "name": "channel_id", "in": "query", "schema": { "type": "integer" } }
        ],
        "responses": {
          "200": { "description": "Dynamic playlist with ordered tracks" },
          "400": { "description": "Invalid mix key" }
        }
      }
    },
    "/admin/access/policy": {
      "get": {
        "tags": ["Access Control Policies"],
        "summary": "Get Access Control Policy",
        "description": "Returns current registration mode (open, invite, allowlist, closed), Telegram membership enforcement flag, and required channels.",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "responses": { "200": { "description": "Current policy" } }
      },
      "patch": {
        "tags": ["Access Control Policies"],
        "summary": "Update Access Control Policy",
        "description": "Updates registration mode, Telegram membership enforcement, lock message, or required chats.",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "registration_mode": { "type": "string", "enum": ["open", "invite", "allowlist", "closed"] },
                  "enforce_membership": { "type": "boolean" },
                  "lock_message": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Updated policy" } }
      }
    },
    "/admin/access/required-chats": {
      "post": {
        "tags": ["Access Control Policies"],
        "summary": "Add Required Telegram Chat",
        "description": "Registers a Telegram channel or group that users must join before gaining access.",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["chat_id"],
                "properties": {
                  "chat_id": { "type": "integer" },
                  "title": { "type": "string" },
                  "invite_link": { "type": "string" },
                  "is_private": { "type": "boolean" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Chat added to required policy" } }
      },
      "delete": {
        "tags": ["Access Control Policies"],
        "summary": "Remove Required Telegram Chat",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "chat_id", "in": "query", "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "Chat removed" } }
      }
    },
    "/admin/access/required-chats/{chat_id}": {
      "delete": {
        "tags": ["Access Control Policies"],
        "summary": "Remove Required Telegram Chat by ID",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "chat_id", "in": "path", "required": true, "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "Chat removed" } }
      }
    },
    "/admin/access/invites": {
      "get": {
        "tags": ["Access Control Policies"],
        "summary": "List Invite Codes",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "include_dead", "in": "query", "schema": { "type": "boolean", "default": false } }
        ],
        "responses": { "200": { "description": "List of active or all invite codes" } }
      },
      "post": {
        "tags": ["Access Control Policies"],
        "summary": "Create Registration Invite Code",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "max_uses": { "type": "integer", "default": 1 },
                  "ttl_days": { "type": "integer", "default": 7 },
                  "note": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Created invite code" } }
      }
    },
    "/admin/access/invites/{code}": {
      "delete": {
        "tags": ["Access Control Policies"],
        "summary": "Revoke Invite Code",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "code", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Invite revoked" } }
      }
    },
    "/admin/access/allowlist": {
      "get": {
        "tags": ["Access Control Policies"],
        "summary": "List Allowed Users",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "responses": { "200": { "description": "List of allowlisted user IDs" } }
      },
      "post": {
        "tags": ["Access Control Policies"],
        "summary": "Add User to Allowlist",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["user_id"],
                "properties": {
                  "user_id": { "type": "integer" },
                  "note": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "User added to allowlist" } }
      }
    },
    "/admin/access/allowlist/{user_id}": {
      "delete": {
        "tags": ["Access Control Policies"],
        "summary": "Remove User from Allowlist",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "user_id", "in": "path", "required": true, "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "User removed from allowlist" } }
      }
    },
    "/admin/access/bypass": {
      "get": {
        "tags": ["Access Control Policies"],
        "summary": "List Membership Bypass Users",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "responses": { "200": { "description": "List of bypassed user IDs" } }
      },
      "post": {
        "tags": ["Access Control Policies"],
        "summary": "Add User to Bypass List",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["user_id"],
                "properties": {
                  "user_id": { "type": "integer" },
                  "note": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "User added to bypass list" } }
      }
    },
    "/admin/access/bypass/{user_id}": {
      "delete": {
        "tags": ["Access Control Policies"],
        "summary": "Remove User from Bypass List",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "user_id", "in": "path", "required": true, "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "User removed from bypass list" } }
      }
    },
    "/admin/access/users": {
      "get": {
        "tags": ["Access Control Policies"],
        "summary": "List / Search User Accounts",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "q", "in": "query", "schema": { "type": "string" } },
          { "name": "status", "in": "query", "schema": { "type": "string", "enum": ["active", "locked", "restricted"] } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } },
          { "name": "skip", "in": "query", "schema": { "type": "integer", "default": 0 } }
        ],
        "responses": { "200": { "description": "User list with total count" } }
      }
    },
    "/admin/access/users/{id}/lock": {
      "post": {
        "tags": ["Access Control Policies"],
        "summary": "Lock User Account",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "integer" } }
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "reason": { "type": "string" },
                  "revoke_sessions": { "type": "boolean", "default": true }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "User locked" } }
      }
    },
    "/admin/access/users/{id}/unlock": {
      "post": {
        "tags": ["Access Control Policies"],
        "summary": "Unlock User Account",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "User unlocked" } }
      }
    },
    "/admin/access/users/{id}/revoke-sessions": {
      "post": {
        "tags": ["Access Control Policies"],
        "summary": "Revoke User Active Sessions",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "All active JWT sessions revoked" } }
      }
    },
    "/admin/access/reverify": {
      "post": {
        "tags": ["Access Control Policies"],
        "summary": "Reverify Telegram Chat Memberships",
        "description": "Tests or reverifies whether target user (or caller) meets all required Telegram chat memberships.",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "user_id": { "type": "integer" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Verification result for each channel" } }
      }
    },
    "/share/{id}": {
      "get": {
        "tags": ["Rich Embeds & Share Cards"],
        "summary": "OpenGraph / Twitter Preview Card (/share/{id})",
        "description": "Renders high-fidelity HTML OpenGraph & Twitter preview cards with dark player UI for social media platforms (Telegram, Discord, Twitter, WhatsApp), or JSON metadata if requested.",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } },
          { "name": "format", "in": "query", "schema": { "type": "string", "enum": ["html", "json"] } }
        ],
        "responses": {
          "200": { "description": "HTML rich card or JSON metadata" },
          "404": { "description": "Track or playlist not found" }
        }
      }
    },
    "/share/tracks/{id}": {
      "get": {
        "tags": ["Rich Embeds & Share Cards"],
        "summary": "Track Share Card Preview",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Track OpenGraph preview" } }
      }
    },
    "/share/playlists/{id}": {
      "get": {
        "tags": ["Rich Embeds & Share Cards"],
        "summary": "Playlist Share Card Preview",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Playlist OpenGraph preview" } }
      }
    },
    "/auth/setup/status": {
      "get": {
        "tags": ["Authentication"],
        "summary": "Initial Setup Status",
        "description": "Checks if the server requires initial admin password configuration or if owner accounts exist.",
        "responses": { "200": { "description": "Setup status" } }
      }
    },
    "/auth/setup": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Complete Initial Admin Setup",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["password"],
                "properties": { "password": { "type": "string" }, "secret_key": { "type": "string" } }
              }
            }
          }
        },
        "responses": { "200": { "description": "Setup completed" } }
      }
    },
    "/auth/password": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Direct Username & Password Login",
        "description": "Direct login without Telegram bot OTP.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["username", "password"],
                "properties": {
                  "username": { "type": "string" },
                  "password": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Login token" } }
      }
    },
    "/auth/password/change": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Change User Password",
        "security": [{ "bearerAuth": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["old_password", "new_password"],
                "properties": {
                  "old_password": { "type": "string" },
                  "new_password": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Password updated" } }
      }
    },
    "/auth/register": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Register New Account",
        "description": "Initiates user registration by validating user credentials and dispatching a 6-digit OTP code to the Telegram account via the bot.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["userid", "username", "password"],
                "properties": {
                  "userid": { "type": "integer" },
                  "username": { "type": "string" },
                  "password": { "type": "string" },
                  "invite_code": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "OTP sent via Telegram bot" } }
      }
    },
    "/auth/create": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Alias for Direct User Creation",
        "responses": { "200": { "description": "Account created" } }
      }
    },
    "/auth/validate": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Validate Telegram Registration OTP",
        "description": "Verifies the 6-digit OTP sent to Telegram user by the bot, completes registration, and returns authentication token.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["userid", "otp"],
                "properties": {
                  "userid": { "type": "integer" },
                  "otp": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Registration completed and token issued" } }
      }
    },
    "/auth/guest": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Guest Login with Guest Password",
        "description": "Authenticates using configured GUEST_PASSWORD and returns a guest session token.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["guest_password"],
                "properties": { "guest_password": { "type": "string" } }
              }
            }
          }
        },
        "responses": { "200": { "description": "Guest session token" } }
      }
    },
    "/auth/telegram": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Telegram WebApp Login",
        "description": "Verifies Telegram initData HMAC-SHA256 signature and returns user session token.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["init_data"],
                "properties": { "init_data": { "type": "string" } }
              }
            }
          }
        },
        "responses": { "200": { "description": "Authentication successful" } }
      }
    },
    "/auth/telegram/widget": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Telegram Widget Login",
        "responses": { "200": { "description": "Authentication successful" } }
      }
    },
    "/auth/me": {
      "get": {
        "tags": ["Authentication"],
        "summary": "Current User Profile",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "responses": {
          "200": { "description": "Current user profile" },
          "401": { "description": "Unauthorized" }
        }
      }
    },
    "/auth/fcm-token": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Update FCM Push Token",
        "description": "Registers or updates the user Firebase Cloud Messaging device push token matching Api/schemas/auth.py.",
        "security": [{ "bearerAuth": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["fcm_token"],
                "properties": {
                  "fcm_token": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": {
          "200": { "description": "Token updated" },
          "401": { "description": "Unauthorized" }
        }
      }
    },
    "/auth/telegram/bot-session": {
      "post": {
        "tags": ["Authentication"],
        "summary": "Create Telegram Bot Auth Session",
        "description": "Creates a temporary authentication session to log in directly via the Telegram bot matching Api/schemas/auth.py.",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "invite_code": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Bot session created" } }
      }
    },
    "/auth/telegram/bot-session/status": {
      "get": {
        "tags": ["Authentication"],
        "summary": "Check Telegram Bot Auth Session Status",
        "description": "Polls status of temporary bot auth session.",
        "parameters": [
          { "name": "session_id", "in": "query", "required": true, "schema": { "type": "string" } },
          { "name": "set_cookie", "in": "query", "schema": { "type": "boolean", "default": true } }
        ],
        "responses": { "200": { "description": "Session status" } }
      }
    },
    "/discord/presence": {
      "post": {
        "tags": ["External Integrations"],
        "summary": "Generate Discord Rich Presence Payload",
        "description": "Formats current track playback into Discord Rich Presence activity object.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["track_id"],
                "properties": {
                  "track_id": { "type": "string" },
                  "title": { "type": "string" },
                  "artist": { "type": "string" },
                  "duration_sec": { "type": "integer" },
                  "elapsed_sec": { "type": "integer" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Discord presence payload" } }
      }
    },
    "/sources/channels": {
      "get": {
        "tags": ["Sources"],
        "summary": "List Monitored Telegram Channels",
        "responses": { "200": { "description": "Channel list" } }
      },
      "post": {
        "tags": ["Sources"],
        "summary": "Add Monitored Channel",
        "security": [{ "bearerAuth": [] }, { "guestPassword": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["channel_id"],
                "properties": { "channel_id": { "type": "integer" }, "title": { "type": "string" } }
              }
            }
          }
        },
        "responses": { "200": { "description": "Channel added" } }
      }
    }
  }
}`

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>StreamGO — API Documentation</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
  <style>
    /* ─── Base ─── */
    :root {
      --bg:       #0a0e17;
      --surface:  #111827;
      --surface2: #1e293b;
      --border:   #1e293b;
      --text:     #e2e8f0;
      --text2:    #94a3b8;
      --accent:   #818cf8;
      --accent2:  #6366f1;
      --get:      #34d399;
      --get-bg:   rgba(52,211,153,0.08);
      --post:     #60a5fa;
      --post-bg:  rgba(96,165,250,0.08);
      --put:      #fbbf24;
      --put-bg:   rgba(251,191,36,0.08);
      --patch:    #c084fc;
      --patch-bg: rgba(192,132,252,0.08);
      --delete:   #f87171;
      --del-bg:   rgba(248,113,113,0.08);
    }
    * { box-sizing: border-box; }
    html { overflow-y: scroll; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font-family: 'Inter', -apple-system, system-ui, sans-serif;
    }

    /* ─── Custom Header ─── */
    .api-header {
      background: var(--surface);
      border-bottom: 1px solid var(--border);
      padding: 1rem 2rem;
      display: flex;
      align-items: center;
      gap: 12px;
    }
    .api-header .dot {
      width: 10px; height: 10px; border-radius: 50%;
      background: var(--accent);
      box-shadow: 0 0 12px var(--accent);
    }
    .api-header h1 {
      font-size: 1.1rem;
      font-weight: 700;
      letter-spacing: -0.01em;
      margin: 0;
      color: #fff;
    }
    .api-header .version {
      font-size: 0.7rem;
      font-weight: 600;
      padding: 2px 8px;
      border-radius: 6px;
      background: rgba(129,140,248,0.12);
      color: var(--accent);
    }

    /* ─── Hide built-in topbar ─── */
    .swagger-ui .topbar { display: none !important; }

    /* ─── Overall theme ─── */
    .swagger-ui .wrapper { max-width: 1200px; }
    .swagger-ui { color: var(--text); font-family: 'Inter', sans-serif; }
    .swagger-ui .info { margin: 32px 0 24px; }
    .swagger-ui .info .title { color: #fff; font-weight: 700; font-size: 1.4rem; }
    .swagger-ui .info .description, .swagger-ui .info .description p { color: var(--text2); }
    .swagger-ui .info a { color: var(--accent); }

    /* ─── Scheme / Server selector ─── */
    .swagger-ui .scheme-container {
      background: var(--surface) !important;
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 12px 16px;
      box-shadow: none;
    }
    .swagger-ui .scheme-container label { color: var(--text2); }
    .swagger-ui select {
      background: var(--surface2); color: var(--text);
      border: 1px solid var(--border); border-radius: 8px;
    }

    /* ─── Tag Groups ─── */
    .swagger-ui .opblock-tag {
      color: #fff !important;
      font-family: 'Inter', sans-serif;
      font-weight: 700 !important;
      border-bottom: 1px solid var(--border) !important;
    }
    .swagger-ui .opblock-tag:hover { background: rgba(255,255,255,0.02); }
    .swagger-ui .opblock-tag svg { fill: var(--text2) !important; }
    .swagger-ui .opblock-tag small { color: var(--text2); }

    /* ─── Operation blocks ─── */
    .swagger-ui .opblock {
      background: var(--surface) !important;
      border: 1px solid var(--border) !important;
      border-radius: 12px !important;
      box-shadow: none !important;
      margin-bottom: 8px;
    }
    .swagger-ui .opblock .opblock-summary {
      border-bottom: none !important;
      padding: 10px 16px;
    }
    .swagger-ui .opblock .opblock-summary-method {
      border-radius: 8px !important;
      font-family: 'JetBrains Mono', monospace !important;
      font-weight: 600 !important;
      font-size: 0.7rem !important;
      padding: 6px 12px !important;
      min-width: 64px;
      text-align: center;
    }
    .swagger-ui .opblock .opblock-summary-path,
    .swagger-ui .opblock .opblock-summary-path a {
      color: var(--text) !important;
      font-family: 'JetBrains Mono', monospace !important;
      font-size: 0.85rem;
    }
    .swagger-ui .opblock .opblock-summary-description {
      color: var(--text2) !important;
      font-size: 0.8rem;
    }

    /* Method colors */
    .swagger-ui .opblock-get { border-left: 3px solid var(--get) !important; }
    .swagger-ui .opblock-get .opblock-summary-method { background: var(--get) !important; color: #000 !important; }
    .swagger-ui .opblock-get .opblock-summary { background: var(--get-bg) !important; border-radius: 11px; }

    .swagger-ui .opblock-post { border-left: 3px solid var(--post) !important; }
    .swagger-ui .opblock-post .opblock-summary-method { background: var(--post) !important; color: #000 !important; }
    .swagger-ui .opblock-post .opblock-summary { background: var(--post-bg) !important; border-radius: 11px; }

    .swagger-ui .opblock-put { border-left: 3px solid var(--put) !important; }
    .swagger-ui .opblock-put .opblock-summary-method { background: var(--put) !important; color: #000 !important; }
    .swagger-ui .opblock-put .opblock-summary { background: var(--put-bg) !important; border-radius: 11px; }

    .swagger-ui .opblock-patch { border-left: 3px solid var(--patch) !important; }
    .swagger-ui .opblock-patch .opblock-summary-method { background: var(--patch) !important; color: #000 !important; }
    .swagger-ui .opblock-patch .opblock-summary { background: var(--patch-bg) !important; border-radius: 11px; }

    .swagger-ui .opblock-delete { border-left: 3px solid var(--delete) !important; }
    .swagger-ui .opblock-delete .opblock-summary-method { background: var(--delete) !important; color: #000 !important; }
    .swagger-ui .opblock-delete .opblock-summary { background: var(--del-bg) !important; border-radius: 11px; }

    /* ─── Expanded operation body ─── */
    .swagger-ui .opblock-body { background: var(--surface) !important; }
    .swagger-ui .opblock-description-wrapper,
    .swagger-ui .opblock-external-docs-wrapper,
    .swagger-ui .opblock-section-header {
      background: transparent !important;
      border-bottom: 1px solid var(--border);
    }
    .swagger-ui .opblock-section-header h4 { color: #fff; }
    .swagger-ui .opblock-section-header label { color: var(--text2); }

    /* ─── Parameters table ─── */
    .swagger-ui table thead tr td, .swagger-ui table thead tr th {
      color: var(--text2) !important;
      border-bottom: 1px solid var(--border) !important;
    }
    .swagger-ui .parameter__name { color: var(--text) !important; font-family: 'JetBrains Mono', monospace; font-size: 0.85rem; }
    .swagger-ui .parameter__name.required::after { color: var(--delete) !important; }
    .swagger-ui .parameter__type { color: var(--accent) !important; font-size: 0.8rem; }
    .swagger-ui .parameter__in { color: var(--text2) !important; }

    /* ─── Input fields ─── */
    .swagger-ui input[type=text], .swagger-ui textarea, .swagger-ui input[type=password], .swagger-ui input[type=search] {
      background: var(--surface2) !important;
      color: var(--text) !important;
      border: 1px solid var(--border) !important;
      border-radius: 8px !important;
      font-family: 'JetBrains Mono', monospace;
      font-size: 0.85rem;
    }
    .swagger-ui input:focus, .swagger-ui textarea:focus {
      border-color: var(--accent) !important;
      outline: none;
      box-shadow: 0 0 0 2px rgba(129,140,248,0.2);
    }

    /* ─── Buttons ─── */
    .swagger-ui .btn {
      border-radius: 8px !important;
      font-family: 'Inter', sans-serif !important;
      font-weight: 600 !important;
    }
    .swagger-ui .btn.execute {
      background: var(--accent2) !important;
      color: #fff !important;
      border: none !important;
    }
    .swagger-ui .btn.execute:hover { background: var(--accent) !important; }
    .swagger-ui .btn.cancel { background: transparent; color: var(--text2) !important; border: 1px solid var(--border) !important; }
    .swagger-ui .btn.authorize {
      color: var(--accent) !important;
      border-color: var(--accent) !important;
    }
    .swagger-ui .btn.authorize svg { fill: var(--accent) !important; }

    /* ─── Response ─── */
    .swagger-ui .responses-inner { background: transparent !important; }
    .swagger-ui .responses-table .response-col_status { color: var(--text) !important; font-weight: 600; }
    .swagger-ui .responses-table .response-col_description { color: var(--text2) !important; }
    .swagger-ui .response-col_description__inner p { color: var(--text2); }

    /* ─── Code highlight / body ─── */
    .swagger-ui .highlight-code,
    .swagger-ui .microlight,
    .swagger-ui pre {
      background: #0d1117 !important;
      color: #c9d1d9 !important;
      border-radius: 10px !important;
      border: 1px solid var(--border);
      font-family: 'JetBrains Mono', monospace !important;
      font-size: 0.82rem;
    }
    .swagger-ui .copy-to-clipboard { background: var(--surface2); border-radius: 8px; }

    /* ─── Model / Schema ─── */
    .swagger-ui .model-container { background: var(--surface) !important; }
    .swagger-ui .model { color: var(--text) !important; font-family: 'JetBrains Mono', monospace; }
    .swagger-ui .model-title { color: var(--accent) !important; }
    .swagger-ui .model .property.primitive { color: var(--text2) !important; }
    .swagger-ui span.model-toggle::after { background: url("data:image/svg+xml;charset=utf-8,%3Csvg xmlns='http://www.w3.org/2000/svg' width='24' height='24' fill='%2394a3b8'%3E%3Cpath d='M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6z'/%3E%3C/svg%3E") center no-repeat; }

    /* ─── Auth Modal ─── */
    .swagger-ui .dialog-ux .modal-ux {
      background: var(--surface) !important;
      border: 1px solid var(--border) !important;
      border-radius: 16px;
      color: var(--text);
    }
    .swagger-ui .dialog-ux .modal-ux-header { border-bottom: 1px solid var(--border); }
    .swagger-ui .dialog-ux .modal-ux-header h3 { color: #fff; }
    .swagger-ui .dialog-ux .modal-ux-content p, .swagger-ui .dialog-ux .modal-ux-content label { color: var(--text2); }
    .swagger-ui .dialog-ux .modal-ux-content .wrapper { border: none; }

    /* ─── Authorize Lock icons ─── */
    .swagger-ui .authorization__btn svg { fill: var(--text2); }
    .swagger-ui .authorization__btn.locked svg { fill: var(--get); }
    .swagger-ui .authorization__btn.unlocked svg { fill: var(--text2); }

    /* ─── Loading ─── */
    .swagger-ui .loading-container .loading::after { color: var(--accent); }

    /* ─── Scrollbar ─── */
    ::-webkit-scrollbar { width: 6px; }
    ::-webkit-scrollbar-track { background: var(--bg); }
    ::-webkit-scrollbar-thumb { background: var(--surface2); border-radius: 3px; }
    ::-webkit-scrollbar-thumb:hover { background: #334155; }
  </style>
</head>
<body>
  <header class="api-header">
    <div class="dot"></div>
    <h1>StreamGO</h1>
    <span class="version">v1.0</span>
  </header>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: "/openapi.json",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout",
        persistAuthorization: true,
        displayRequestDuration: true,
        defaultModelsExpandDepth: -1,
        docExpansion: "list",
        syntaxHighlight: { activated: true, theme: "monokai" }
      });
    };
  </script>
</body>
</html>`

const redocHTML = `<!DOCTYPE html>
<html>
<head>
  <title>StreamGO - ReDoc</title>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
  <style>body { margin: 0; padding: 0; }</style>
</head>
<body>
  <redoc spec-url='/openapi.json'></redoc>
  <script src="https://cdn.jsdelivr.net/npm/redoc@next/bundles/redoc.standalone.js"></script>
</body>
</html>`

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(openAPISpecJSON))
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func (s *Server) handleRedoc(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(redocHTML))
}
