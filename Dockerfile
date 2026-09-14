# syntax=docker/dockerfile:1.2

FROM node:20-alpine AS frontend-builder

WORKDIR /app/WebX

COPY WebX/package*.json ./
RUN npm install
COPY WebX/ .

RUN npm run build

FROM python:3.12-slim-bookworm

RUN apt-get update && \
    apt-get install -y \
      git \
      openssh-client \
      build-essential \
      gcc \
      wget \
      curl \
      dpkg \
      ca-certificates \
      gnupg \
      aria2 \
      mediainfo \
      ffmpeg \
      libavcodec-extra \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

RUN python3 -m venv /app/streamvenv

COPY requirements.txt .
RUN /app/streamvenv/bin/pip install --no-cache-dir --upgrade pip setuptools wheel && \
    /app/streamvenv/bin/pip install --no-cache-dir -r requirements.txt

COPY . .

COPY --from=frontend-builder /app/WebX/dist ./dist

COPY start.sh /app/start.sh
RUN chmod +x /app/start.sh

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PORT=8000

EXPOSE 8000

CMD ["bash", "/app/start.sh"]