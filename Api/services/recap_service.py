"""
Recap engine — turns listening events into a standardized, cached RecapSnapshot.

Data sources (in priority order):
  1. ``listening_events`` — rich events posted by clients (played_ms, completed, skipped …)
  2. ``userHistory``     — legacy "stream started" rows, used as a fallback for older data

Play rules (kept consistent everywhere):
    < 30 s          → ignored (counts as a skip)
    ≥ 30 s          → play
    ≥ 50 %          → strong play
    ≥ 80 % / flag   → completed
"""
from __future__ import annotations

import calendar
import datetime as dt
import secrets
import time
from collections import Counter, defaultdict
from typing import Any, Iterable, Literal, Optional

from Api.deps.db import get_audio_tracks_collection
from Api.services.track_service import _browse_item_from_doc
from stream.database.MongoDb import db_handler

SCHEMA_VERSION = 1
PeriodType = Literal["weekly", "monthly", "yearly"]
MIN_PLAY_MS = 30_000
NIGHT_HOURS = {21, 22, 23, 0, 1, 2, 3}
EARLY_HOURS = {5, 6, 7, 8}
SESSION_GAP_SEC = 30 * 60


def _events_col():
    return db_handler.get_collection("listening_events").collection


def _history_col():
    return db_handler.get_collection("userHistory").collection


def _snapshots_col():
    return db_handler.get_collection("recap_snapshots").collection


def _shares_col():
    return db_handler.get_collection("recap_shares").collection


# --------------------------------------------------------------------------- periods


def _local_midnight_utc(year: int, month: int, day: int, tz_offset_min: int) -> float:
    """Epoch (UTC) of local midnight for the given date, given the client's UTC offset in minutes."""
    local = dt.datetime(year, month, day, tzinfo=dt.timezone.utc)
    return local.timestamp() - tz_offset_min * 60


def period_bounds(ptype: PeriodType, period: str, tz_offset_min: int = 0) -> tuple[float, float, str]:
    """Return (start_epoch, end_epoch, human_label) for a period id."""
    try:
        if ptype == "weekly":
            year_s, week_s = period.upper().split("-W")
            year, week = int(year_s), int(week_s)
            monday = dt.date.fromisocalendar(year, week, 1)
            start = _local_midnight_utc(monday.year, monday.month, monday.day, tz_offset_min)
            nxt = monday + dt.timedelta(days=7)
            end = _local_midnight_utc(nxt.year, nxt.month, nxt.day, tz_offset_min)
            sunday = monday + dt.timedelta(days=6)
            label = f"{monday.strftime('%b %-d')} – {sunday.strftime('%b %-d, %Y')}"
            return start, end, label
        if ptype == "monthly":
            year, month = (int(x) for x in period.split("-"))
            start = _local_midnight_utc(year, month, 1, tz_offset_min)
            ny, nm = (year + 1, 1) if month == 12 else (year, month + 1)
            end = _local_midnight_utc(ny, nm, 1, tz_offset_min)
            return start, end, f"{calendar.month_name[month]} {year}"
        if ptype == "yearly":
            year = int(period)
            return _local_midnight_utc(year, 1, 1, tz_offset_min), _local_midnight_utc(year + 1, 1, 1, tz_offset_min), str(year)
    except Exception as exc:  # pragma: no cover - validation
        raise ValueError(f"invalid period '{period}' for {ptype}") from exc
    raise ValueError("unknown period type")


def previous_period(ptype: PeriodType, period: str) -> str:
    if ptype == "weekly":
        year_s, week_s = period.upper().split("-W")
        monday = dt.date.fromisocalendar(int(year_s), int(week_s), 1) - dt.timedelta(days=7)
        iso = monday.isocalendar()
        return f"{iso[0]}-W{iso[1]:02d}"
    if ptype == "monthly":
        year, month = (int(x) for x in period.split("-"))
        return f"{year - 1}-12" if month == 1 else f"{year}-{month - 1:02d}"
    return str(int(period) - 1)


def period_id_for(ptype: PeriodType, when: dt.datetime) -> str:
    if ptype == "weekly":
        iso = when.isocalendar()
        return f"{iso[0]}-W{iso[1]:02d}"
    if ptype == "monthly":
        return f"{when.year}-{when.month:02d}"
    return str(when.year)


def is_period_available(ptype: PeriodType, period: str, tz_offset_min: int = 0) -> bool:
    """Check if a recap period is ready/unlocked to show to users.

    Rules:
    - Completed periods (end <= now) are always available.
    - Ongoing weekly recaps only show on the weekend (Saturday or Sunday, e.g. Sunday).
    - Ongoing monthly recaps only show on the last day of the month.
    - Ongoing yearly recaps only show on Dec 31st.
    """
    start, end, _ = period_bounds(ptype, period, tz_offset_min)
    now = time.time()
    if end <= now:
        return True

    now_local = _local_dt(now, tz_offset_min)
    if ptype == "weekly":
        # Every weekend (Saturday or Sunday)
        return now_local.weekday() in (5, 6)
    if ptype == "monthly":
        # Last day of the month
        _, last_day = calendar.monthrange(now_local.year, now_local.month)
        return now_local.day == last_day
    if ptype == "yearly":
        # 31st Dec
        return now_local.month == 12 and now_local.day == 31
    return False


# --------------------------------------------------------------------------- events


async def record_events(user_id: int, events: list[dict[str, Any]]) -> int:
    """Idempotent batch insert (client supplies the event id)."""
    col = _events_col()
    stored = 0
    now = time.time()
    for ev in events:
        eid = str(ev.get("id") or "").strip()
        tid = str(ev.get("track_id") or "").strip()
        if not eid or not tid:
            continue
        played_ms = max(0, int(ev.get("played_ms") or 0))
        duration_ms = max(0, int(ev.get("duration_ms") or 0))
        doc = {
            "_id": f"{int(user_id)}:{eid}",
            "user_id": int(user_id),
            "track_id": tid,
            "played_at": float(ev.get("played_at") or now),
            "started_at": float(ev.get("started_at") or ev.get("played_at") or now),
            "played_ms": played_ms,
            "duration_ms": duration_ms,
            "completed": bool(ev.get("completed")),
            "skipped": bool(ev.get("skipped")),
            "source": str(ev.get("source") or "server")[:32],
            "session_id": (str(ev.get("session_id"))[:64] if ev.get("session_id") else None),
            "created_at": now,
        }
        try:
            res = await col.update_one({"_id": doc["_id"]}, {"$setOnInsert": doc}, upsert=True)
            if res.upserted_id is not None:
                stored += 1
        except Exception:
            continue
    return stored


async def _load_events(user_id: int, start: float, end: float) -> list[dict[str, Any]]:
    q = {"user_id": int(user_id), "played_at": {"$gte": start, "$lt": end}}
    events: list[dict[str, Any]] = []
    async for doc in _events_col().find(q):
        events.append(doc)
    if events:
        return events
    # Fallback: legacy history rows (one row = one stream start). Duration is filled in later.
    hq = {"$or": [{"user_id": int(user_id)}, {"user_id": str(user_id)}], "played_at": {"$gte": start, "$lt": end}}
    async for doc in _history_col().find(hq):
        events.append(
            {
                "track_id": str(doc.get("track_id") or ""),
                "played_at": float(doc.get("played_at") or 0),
                "started_at": float(doc.get("played_at") or 0),
                "played_ms": None,  # unknown → assume full play
                "duration_ms": 0,
                "completed": False,
                "skipped": False,
                "legacy": True,
            }
        )
    return [e for e in events if e.get("track_id")]


async def _load_tracks(track_ids: Iterable[str]) -> dict[str, dict[str, Any]]:
    ids = list({str(t) for t in track_ids if t})
    if not ids:
        return {}
    col = get_audio_tracks_collection()
    out: dict[str, dict[str, Any]] = {}
    for i in range(0, len(ids), 500):
        chunk = ids[i : i + 500]
        async for doc in col.find({"_id": {"$in": chunk}}, {"_id": 1, "audio": 1, "spotify": 1, "topic_name": 1, "topic_id": 1, "updated_at": 1}):
            item = _browse_item_from_doc(doc).model_dump()
            out[item["id"]] = item
    return out


async def _first_seen_before(user_id: int, track_ids: list[str], before: float) -> set[str]:
    if not track_ids:
        return set()
    seen: set[str] = set()
    ev_ids = await _events_col().distinct("track_id", {"user_id": int(user_id), "track_id": {"$in": track_ids}, "played_at": {"$lt": before}})
    seen.update(str(x) for x in ev_ids)
    hist_ids = await _history_col().distinct(
        "track_id", {"$or": [{"user_id": int(user_id)}, {"user_id": str(user_id)}], "track_id": {"$in": track_ids}, "played_at": {"$lt": before}}
    )
    seen.update(str(x) for x in hist_ids)
    return seen


# --------------------------------------------------------------------------- statistics


def _classify(ev: dict[str, Any], duration_ms: int) -> tuple[bool, bool, bool, int]:
    """→ (is_play, is_completed, is_skip, played_ms)"""
    played = ev.get("played_ms")
    if played is None:  # legacy row: assume the track was played through
        played = duration_ms or 3 * 60_000
    played = int(played)
    completed = bool(ev.get("completed")) or (duration_ms > 0 and played >= 0.8 * duration_ms)
    is_play = played >= MIN_PLAY_MS or completed
    is_skip = not is_play
    return is_play, completed, is_skip, min(played, duration_ms) if duration_ms else played


def _local_dt(ts: float, tz_offset_min: int) -> dt.datetime:
    return dt.datetime.fromtimestamp(ts + tz_offset_min * 60, tz=dt.timezone.utc)


def _compute_stats(events: list[dict[str, Any]], tracks: dict[str, dict[str, Any]], tz_offset_min: int, seen_before: set[str]) -> dict[str, Any]:
    total_ms = 0
    plays = completed = skipped = 0
    track_plays: Counter[str] = Counter()
    track_ms: Counter[str] = Counter()
    artist_plays: Counter[str] = Counter()
    artist_ms: Counter[str] = Counter()
    artist_cover: dict[str, str | None] = {}
    album_plays: Counter[str] = Counter()
    album_ms: Counter[str] = Counter()
    album_meta: dict[str, dict[str, Any]] = {}
    topic_plays: Counter[str] = Counter()
    by_hour = [0] * 24
    by_weekday = [0] * 7
    by_date: Counter[str] = Counter()
    by_month_ms: Counter[str] = Counter()
    lossless_plays = 0
    starts: list[tuple[float, int]] = []

    for ev in events:
        t = tracks.get(str(ev.get("track_id")))
        duration_ms = int(ev.get("duration_ms") or 0) or int((t or {}).get("duration_sec") or 0) * 1000
        is_play, is_completed, is_skip, played_ms = _classify(ev, duration_ms)
        if is_skip:
            skipped += 1
            continue
        plays += 1
        if is_completed:
            completed += 1
        total_ms += played_ms
        tid = str(ev["track_id"])
        track_plays[tid] += 1
        track_ms[tid] += played_ms
        when = _local_dt(float(ev.get("played_at") or 0), tz_offset_min)
        by_hour[when.hour] += played_ms
        by_weekday[when.weekday()] += played_ms
        by_date[when.strftime("%Y-%m-%d")] += played_ms
        by_month_ms[when.strftime("%Y-%m")] += played_ms
        starts.append((float(ev.get("started_at") or ev.get("played_at") or 0), played_ms))
        if t:
            artist = (t.get("artist") or "Unknown artist").strip()
            artist_plays[artist] += 1
            artist_ms[artist] += played_ms
            artist_cover.setdefault(artist, t.get("cover_url"))
            if t.get("album"):
                key = f"{t.get('album')}::{artist}"
                album_plays[key] += 1
                album_ms[key] += played_ms
                album_meta.setdefault(key, {"album": t.get("album"), "artist": artist, "album_id": t.get("album_id"), "cover_url": t.get("cover_url")})
            if t.get("topic_name"):
                topic_plays[str(t["topic_name"])] += 1
            if str(t.get("type") or "").upper() in {"FLAC", "ALAC", "WAV"}:
                lossless_plays += 1

    # sessions: sort by start, split on gaps > 30 min
    longest_session_ms = 0
    session_count = 0
    if starts:
        starts.sort()
        cur_start, cur_end = starts[0][0], starts[0][0] + starts[0][1] / 1000
        cur_ms = starts[0][1]
        session_count = 1
        for s0, ms in starts[1:]:
            if s0 - cur_end > SESSION_GAP_SEC:
                longest_session_ms = max(longest_session_ms, cur_ms)
                cur_start, cur_ms = s0, 0
                session_count += 1
            cur_ms += ms
            cur_end = max(cur_end, s0 + ms / 1000)
        longest_session_ms = max(longest_session_ms, cur_ms)

    def top_tracks(n: int) -> list[dict[str, Any]]:
        out = []
        for tid, cnt in track_plays.most_common(n):
            t = tracks.get(tid) or {"id": tid, "title": "Unknown track", "artist": None}
            out.append({"track": t, "plays": cnt, "minutes": round(track_ms[tid] / 60_000, 1)})
        return out

    def top_artists(n: int) -> list[dict[str, Any]]:
        return [{"name": a, "plays": c, "minutes": round(artist_ms[a] / 60_000, 1), "cover_url": artist_cover.get(a)} for a, c in artist_plays.most_common(n)]

    def top_albums(n: int) -> list[dict[str, Any]]:
        return [{**album_meta[k], "plays": c, "minutes": round(album_ms[k] / 60_000, 1)} for k, c in album_plays.most_common(n)]

    unique_tracks = len(track_plays)
    night_ms = sum(by_hour[h] for h in NIGHT_HOURS)
    early_ms = sum(by_hour[h] for h in EARLY_HOURS)
    weekend_ms = by_weekday[5] + by_weekday[6]
    repeat_plays = sum(c - 1 for c in track_plays.values() if c > 1)
    most_active_hour = max(range(24), key=lambda h: by_hour[h]) if total_ms else None
    most_active_weekday = max(range(7), key=lambda d: by_weekday[d]) if total_ms else None
    most_active_date = by_date.most_common(1)[0][0] if by_date else None
    discovery = [tid for tid in track_plays if tid not in seen_before]

    return {
        "totalMinutes": round(total_ms / 60_000),
        "totalPlays": plays,
        "completedPlays": completed,
        "skippedPlays": skipped,
        "uniqueTracks": unique_tracks,
        "uniqueArtists": len(artist_plays),
        "uniqueAlbums": len(album_plays),
        "topTracks": top_tracks(10),
        "topArtists": top_artists(10),
        "topAlbums": top_albums(6),
        "topGenres": [{"name": n, "plays": c} for n, c in topic_plays.most_common(5)],
        "listeningByHour": [round(ms / 60_000) for ms in by_hour],
        "listeningByWeekday": [round(ms / 60_000) for ms in by_weekday],
        "listeningByDay": [{"date": d, "minutes": round(ms / 60_000)} for d, ms in sorted(by_date.items())],
        "listeningByMonth": [{"month": m, "minutes": round(ms / 60_000)} for m, ms in sorted(by_month_ms.items())],
        "discoveryCount": len(discovery),
        "discoveryTracks": [tracks.get(tid) for tid in discovery[:6] if tracks.get(tid)],
        "repeatCount": repeat_plays,
        "mostActiveHour": most_active_hour,
        "mostActiveWeekday": most_active_weekday,
        "mostActiveDate": most_active_date,
        "mostActiveDateMinutes": round(by_date[most_active_date] / 60_000) if most_active_date else 0,
        "longestSessionMinutes": round(longest_session_ms / 60_000),
        "sessionCount": session_count,
        "nightShare": round(night_ms / total_ms, 3) if total_ms else 0,
        "earlyShare": round(early_ms / total_ms, 3) if total_ms else 0,
        "weekendShare": round(weekend_ms / total_ms, 3) if total_ms else 0,
        "losslessShare": round(lossless_plays / plays, 3) if plays else 0,
        "topArtistShare": round(artist_plays.most_common(1)[0][1] / plays, 3) if plays and artist_plays else 0,
    }


# --------------------------------------------------------------------------- personality


def _personality(stats: dict[str, Any]) -> list[dict[str, Any]]:
    """Deterministic, transparent classifications. Each entry explains its rule."""
    out: list[dict[str, Any]] = []
    plays = stats["totalPlays"]
    if plays < 5:
        return out
    if stats["nightShare"] >= 0.45:
        out.append({"id": "night_owl", "name": "Night Owl", "detail": f"{round(stats['nightShare'] * 100)}% of your listening happened after 9 PM"})
    if stats["earlyShare"] >= 0.35:
        out.append({"id": "early_bird", "name": "Early Bird", "detail": f"{round(stats['earlyShare'] * 100)}% of your listening was before 9 AM"})
    if stats["uniqueTracks"] and stats["discoveryCount"] / stats["uniqueTracks"] >= 0.5 and stats["discoveryCount"] >= 10:
        out.append({"id": "explorer", "name": "Explorer", "detail": f"{stats['discoveryCount']} of {stats['uniqueTracks']} tracks were new to you"})
    if stats["repeatCount"] / plays >= 0.45:
        out.append({"id": "replayer", "name": "Replayer", "detail": f"{stats['repeatCount']} replays — you know what you like"})
    if stats["topArtistShare"] >= 0.4 and stats["topArtists"]:
        out.append({"id": "loyalist", "name": "Loyalist", "detail": f"{round(stats['topArtistShare'] * 100)}% of plays were {stats['topArtists'][0]['name']}"})
    if stats["uniqueArtists"] >= 25 and stats["topArtistShare"] < 0.15:
        out.append({"id": "genre_hopper", "name": "Genre Hopper", "detail": f"{stats['uniqueArtists']} artists, no clear favourite"})
    if stats["uniqueAlbums"] and stats["topAlbums"] and stats["topAlbums"][0]["plays"] >= 8 and stats["completedPlays"] / plays >= 0.7:
        out.append({"id": "album_collector", "name": "Album Listener", "detail": "You play albums through, front to back"})
    if stats["weekendShare"] >= 0.5:
        out.append({"id": "weekend", "name": "Weekend Listener", "detail": f"{round(stats['weekendShare'] * 100)}% of listening on weekends"})
    if stats["longestSessionMinutes"] >= 120:
        out.append({"id": "binge", "name": "Binge Listener", "detail": f"Longest session: {stats['longestSessionMinutes']} minutes"})
    if stats["losslessShare"] >= 0.6:
        out.append({"id": "audiophile", "name": "Audiophile", "detail": f"{round(stats['losslessShare'] * 100)}% of plays were lossless"})
    if not out:
        out.append({"id": "steady", "name": "Steady Listener", "detail": f"{plays} plays across {stats['uniqueArtists']} artists"})
    return out[:3]


# --------------------------------------------------------------------------- snapshots


async def build_snapshot(user_id: int, ptype: PeriodType, period: str, tz_offset_min: int = 0) -> dict[str, Any]:
    start, end, label = period_bounds(ptype, period, tz_offset_min)
    events = await _load_events(user_id, start, end)
    tracks = await _load_tracks(e["track_id"] for e in events)
    seen_before = await _first_seen_before(user_id, list({str(e["track_id"]) for e in events}), start)
    stats = _compute_stats(events, tracks, tz_offset_min, seen_before)

    # previous period (light comparison)
    prev_id = previous_period(ptype, period)
    ps, pe, _ = period_bounds(ptype, prev_id, tz_offset_min)
    prev_events = await _load_events(user_id, ps, pe)
    comparison: dict[str, Any] | None = None
    if prev_events:
        prev_tracks = await _load_tracks(e["track_id"] for e in prev_events)
        prev = _compute_stats(prev_events, prev_tracks, tz_offset_min, set())

        def delta(cur: float, old: float) -> float | None:
            return round((cur - old) / old * 100) if old else None

        comparison = {
            "period": prev_id,
            "totalMinutes": prev["totalMinutes"],
            "totalPlays": prev["totalPlays"],
            "uniqueArtists": prev["uniqueArtists"],
            "uniqueTracks": prev["uniqueTracks"],
            "minutesDeltaPct": delta(stats["totalMinutes"], prev["totalMinutes"]),
            "playsDeltaPct": delta(stats["totalPlays"], prev["totalPlays"]),
            "artistsDelta": stats["uniqueArtists"] - prev["uniqueArtists"],
            "nightShareDelta": round((stats["nightShare"] - prev["nightShare"]) * 100),
            "topArtistPrev": prev["topArtists"][0]["name"] if prev["topArtists"] else None,
        }

    now = time.time()
    return {
        "schemaVersion": SCHEMA_VERSION,
        "type": ptype,
        "period": period,
        "label": label,
        "start": start,
        "end": end,
        "ongoing": end > now,
        "generatedAt": now,
        "tzOffsetMin": tz_offset_min,
        "eventCount": len(events),
        "legacyData": bool(events) and all(e.get("legacy") for e in events),
        "stats": stats,
        "personality": _personality(stats),
        "comparison": comparison,
    }


async def get_snapshot(user_id: int, ptype: PeriodType, period: str, tz_offset_min: int = 0, force: bool = False) -> dict[str, Any]:
    key = {"user_id": int(user_id), "type": ptype, "period": period, "tz": int(tz_offset_min)}
    col = _snapshots_col()
    now = time.time()
    if not force:
        cached = await col.find_one(key)
        if cached and cached.get("schemaVersion") == SCHEMA_VERSION:
            snap = cached.get("snapshot") or {}
            ttl = 15 * 60 if snap.get("ongoing") else 24 * 3600
            if now - float(cached.get("generatedAt") or 0) < ttl:
                return snap
    snap = await build_snapshot(user_id, ptype, period, tz_offset_min)
    try:
        await col.update_one(key, {"$set": {**key, "schemaVersion": SCHEMA_VERSION, "generatedAt": now, "snapshot": snap}}, upsert=True)
    except Exception:
        pass
    return snap


async def _has_plays(user_id: int, start: float, end: float) -> bool:
    uid = int(user_id)
    doc = await _events_col().find_one({"user_id": uid, "played_at": {"$gte": start, "$lt": end}}, {"_id": 1})
    if doc:
        return True
    hdoc = await _history_col().find_one(
        {"$or": [{"user_id": uid}, {"user_id": str(uid)}], "played_at": {"$gte": start, "$lt": end}},
        {"_id": 1},
    )
    return bool(hdoc)


async def list_available(user_id: int, tz_offset_min: int = 0) -> list[dict[str, Any]]:
    """Periods that have listening data and are unlocked to show:
    - Weekly: past completed weeks, or ongoing week on the weekend (Saturday/Sunday)
    - Monthly: past completed months, or ongoing month on its last day
    - Yearly: past completed years, or ongoing year on Dec 31st
    """
    first: float | None = None
    doc = await _events_col().find_one({"user_id": int(user_id)}, sort=[("played_at", 1)])
    if doc:
        first = float(doc["played_at"])
    hdoc = await _history_col().find_one({"$or": [{"user_id": int(user_id)}, {"user_id": str(user_id)}]}, sort=[("played_at", 1)])
    if hdoc and hdoc.get("played_at"):
        first = min(first, float(hdoc["played_at"])) if first else float(hdoc["played_at"])
    if not first:
        return []
    now = time.time()
    now_local = _local_dt(now, tz_offset_min)
    first_local = _local_dt(first, tz_offset_min)
    out: list[dict[str, Any]] = []

    # weeks (current + previous, up to 8)
    cur = now_local
    for _ in range(8):
        pid = period_id_for("weekly", cur)
        s, e, label = period_bounds("weekly", pid, tz_offset_min)
        if e < first:
            break
        if is_period_available("weekly", pid, tz_offset_min):
            if await _has_plays(user_id, s, e):
                out.append({"type": "weekly", "period": pid, "label": label, "ongoing": e > now})
        cur -= dt.timedelta(days=7)

    # months (current + previous, up to 12)
    y, m = now_local.year, now_local.month
    for _ in range(12):
        pid = f"{y}-{m:02d}"
        s, e, label = period_bounds("monthly", pid, tz_offset_min)
        if e < first:
            break
        if is_period_available("monthly", pid, tz_offset_min):
            if await _has_plays(user_id, s, e):
                out.append({"type": "monthly", "period": pid, "label": label, "ongoing": e > now})
        y, m = (y - 1, 12) if m == 1 else (y, m - 1)

    # years
    for y in range(now_local.year, first_local.year - 1, -1):
        pid = str(y)
        s, e, label = period_bounds("yearly", pid, tz_offset_min)
        if is_period_available("yearly", pid, tz_offset_min):
            if await _has_plays(user_id, s, e):
                out.append({"type": "yearly", "period": pid, "label": label, "ongoing": e > now})

    return out


# --------------------------------------------------------------------------- sharing


def _public_summary(snap: dict[str, Any]) -> dict[str, Any]:
    st = snap["stats"]
    top_track = st["topTracks"][0]["track"] if st["topTracks"] else None
    return {
        "type": snap["type"],
        "period": snap["period"],
        "label": snap["label"],
        "totalMinutes": st["totalMinutes"],
        "totalPlays": st["totalPlays"],
        "uniqueArtists": st["uniqueArtists"],
        "uniqueTracks": st["uniqueTracks"],
        "topArtist": st["topArtists"][0] if st["topArtists"] else None,
        "topTrack": {"title": top_track.get("title"), "artist": top_track.get("artist"), "cover_url": top_track.get("cover_url")} if top_track else None,
        "personality": snap.get("personality") or [],
    }


async def create_share(user_id: int, snap: dict[str, Any]) -> dict[str, Any]:
    col = _shares_col()
    existing = await col.find_one({"user_id": int(user_id), "type": snap["type"], "period": snap["period"], "revoked_at": None})
    summary = _public_summary(snap)
    if existing:
        await col.update_one({"_id": existing["_id"]}, {"$set": {"summary": summary}})
        return {"token": existing["token"], "created_at": existing["created_at"]}
    token = secrets.token_urlsafe(9)
    doc = {"token": token, "user_id": int(user_id), "type": snap["type"], "period": snap["period"], "created_at": time.time(), "revoked_at": None, "summary": summary}
    await col.insert_one(doc)
    return {"token": token, "created_at": doc["created_at"]}


async def list_shares(user_id: int) -> list[dict[str, Any]]:
    out = []
    async for d in _shares_col().find({"user_id": int(user_id), "revoked_at": None}).sort("created_at", -1):
        out.append({"token": d["token"], "type": d["type"], "period": d["period"], "label": (d.get("summary") or {}).get("label"), "created_at": d["created_at"]})
    return out


async def revoke_share(user_id: int, token: str) -> bool:
    res = await _shares_col().update_one({"user_id": int(user_id), "token": token, "revoked_at": None}, {"$set": {"revoked_at": time.time()}})
    return res.modified_count > 0


async def get_public_share(token: str) -> Optional[dict[str, Any]]:
    d = await _shares_col().find_one({"token": token, "revoked_at": None})
    return d.get("summary") if d else None


async def delete_user_recap_data(user_id: int) -> dict[str, int]:
    uid = int(user_id)
    e = await _events_col().delete_many({"user_id": uid})
    s = await _snapshots_col().delete_many({"user_id": uid})
    sh = await _shares_col().delete_many({"user_id": uid})
    return {"events": e.deleted_count, "snapshots": s.deleted_count, "shares": sh.deleted_count}
