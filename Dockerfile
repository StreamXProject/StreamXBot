# syntax=docker/dockerfile:1.2

FROM node:20-alpine AS frontend-builder

RUN apk add --no-cache git

RUN git clone https://github.com/StreamXProject/WebX.git /app/WebX

WORKDIR /app/WebX

RUN npm install
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
ENV PATH="/app/streamvenv/bin:$PATH" \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PORT=8000

COPY requirements.txt .
RUN pip install --no-cache-dir --upgrade pip setuptools wheel && \
    pip install --no-cache-dir -r requirements.txt

COPY . .

COPY --from=frontend-builder /app/WebX/dist ./dist

EXPOSE 8000

CMD ["python3", "-m", "stream"]