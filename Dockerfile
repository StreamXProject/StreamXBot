# syntax=docker/dockerfile:1.4

# Stage 1: Build WebX SPA Frontend
FROM node:20-alpine AS frontend-builder

RUN apk add --no-cache git

ARG WEBX_REPO=https://github.com/StreamXProject/WebX.git
RUN git clone --depth 1 ${WEBX_REPO} /app/WebX

WORKDIR /app/WebX

RUN npm install
RUN npm run build

# Stage 2: Build StreamGO Backend Binary
FROM golang:1.25-alpine AS go-builder

RUN apk add --no-cache git ca-certificates

ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/server ./cmd/server

# Stage 3: Production Runtime
FROM alpine:3.21

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    ffmpeg \
    mediainfo

WORKDIR /app

# Copy compiled Go server binary
COPY --from=go-builder /app/bin/server /app/server

# Copy compiled WebX SPA frontend
COPY --from=frontend-builder /app/WebX/dist /app/dist

# Default configuration environment variables
ENV PORT=8000 \
    APP_ENV=production

EXPOSE 8000

ENTRYPOINT ["/app/server"]
