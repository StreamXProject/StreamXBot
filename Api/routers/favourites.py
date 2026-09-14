import time
from typing import Optional
from pydantic import BaseModel
from pymongo import UpdateOne

from fastapi import APIRouter, Depends, HTTPException, Query

from Api.schemas.favourites import ArtistFavouriteCreate, FavouriteCreate, FavouriteIdsResponse, FavouritesResponse, FavouriteItem
from Api.schemas.browse import BrowseResponse
from Api.services.track_service import get_track_by_id, get_tracks_by_ids, user_top_played_tracks
from Api.utils.auth import get_optional_user_id, require_user_id
from stream.database.MongoDb import db_handler


router = APIRouter(prefix="/me", tags=["me"])


@router.post("/favourites")
async def add_favourite(payload: FavouriteCreate, user_id: int = Depends(require_user_id)):
    track_id = (payload.track_id or "").strip()
    if not track_id:
        raise HTTPException(status_code=400, detail="track_id is required")

    track = await get_track_by_id(track_id)
    if not track:
        raise HTTPException(status_code=404, detail="track not found")

    col = db_handler.get_collection("user_favourites").collection
    res = await col.update_one(
        {"user_id": int(user_id), "track_id": track_id},
        {
            "$setOnInsert": {"created_at": time.time()},
            "$set": {"user_id": int(user_id), "track_id": track_id, "updated_at": time.time()},
        },
        upsert=True,
    )
    return {"ok": True, "already_exists": res.upserted_id is None}


@router.delete("/favourites/{track_id}")
async def remove_favourite(track_id: str, user_id: int = Depends(require_user_id)):
    track_id = (track_id or "").strip()
    if not track_id:
        raise HTTPException(status_code=400, detail="track_id is required")

    col = db_handler.get_collection("user_favourites").collection
    res = await col.delete_one({"user_id": int(user_id), "track_id": track_id})
    return {"ok": True, "deleted": bool(getattr(res, "deleted_count", 0))}


@router.get("/favourites", response_model=FavouritesResponse)
async def list_favourites(
    user_id: int = Depends(require_user_id),
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=20, ge=1, le=100),
):
    page = int(page)
    per_page = int(limit)
    skip = (page - 1) * per_page

    col = db_handler.get_collection("user_favourites").collection
    query = {"user_id": int(user_id)}
    total = await col.count_documents(query)

    cursor = (
        col.find(query, {"_id": 0, "track_id": 1, "created_at": 1, "updated_at": 1})
        .sort([("created_at", -1)])
        .skip(skip)
        .limit(per_page)
    )

    fav_rows: list[dict] = []
    last_ts: float | None = None
    async for doc in cursor:
        tid = (doc.get("track_id") or "").strip()
        if not tid:
            continue
        fav_rows.append({"track_id": tid, "created_at": doc.get("created_at")})
        ts = doc.get("updated_at")
        if ts is None:
            ts = doc.get("created_at")
        if isinstance(ts, (int, float)):
            if last_ts is None or float(ts) > float(last_ts):
                last_ts = float(ts)

    tracks = await get_tracks_by_ids([r["track_id"] for r in fav_rows], user_id=user_id)
    by_id = {str(t.get("_id")): t for t in tracks if t.get("_id")}
    items: list[FavouriteItem] = []
    for r in fav_rows:
        t = by_id.get(r["track_id"])
        if not t:
            continue
        items.append(FavouriteItem(track=t, created_at=r.get("created_at")))
    return FavouritesResponse(page=page, per_page=per_page, total=total, items=items, last_updated_at=last_ts)


@router.get("/favourites/ids", response_model=FavouriteIdsResponse)
async def list_favourite_ids(
    user_id: Optional[int] = Depends(get_optional_user_id),
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=200, ge=1, le=1000),
):
    if user_id is None:
        return FavouriteIdsResponse(page=page, per_page=limit, total=0, ids=[], exists=False)
    page = int(page)
    per_page = int(limit)
    skip = (page - 1) * per_page

    col = db_handler.get_collection("user_favourites").collection
    query = {"user_id": user_id}
    total = await col.count_documents(query)

    cursor = (
        col.find(query, {"_id": 0, "track_id": 1, "updated_at": 1, "created_at": 1})
        .sort([("created_at", -1)])
        .skip(skip)
        .limit(per_page)
    )

    ids: list[str] = []
    last_ts: float | None = None
    async for doc in cursor:
        tid = (doc.get("track_id") or "").strip()
        if tid:
            ids.append(tid)
            ts = doc.get("updated_at")
            if ts is None:
                ts = doc.get("created_at")
            if isinstance(ts, (int, float)):
                if last_ts is None or float(ts) > float(last_ts):
                    last_ts = float(ts)

    return FavouriteIdsResponse(
        page=page,
        per_page=per_page,
        total=total,
        ids=ids,
        exists=bool(total > 0),
        last_updated_at=last_ts,
    )


@router.get("/top-played", response_model=BrowseResponse)
async def my_top_played(
    user_id: int = Depends(require_user_id),
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=50, ge=1, le=100),
):
    return await user_top_played_tracks(user_id=int(user_id), page=int(page), per_page=int(limit))


@router.post("/artists/favourites")
@router.post("/artist/favorite")
@router.post("/artists/favorite")
async def add_artist_favourite(
    payload: Optional[ArtistFavouriteCreate] = None,
    artist_id: Optional[str] = Query(default=None),
    id: Optional[str] = Query(default=None),
    user_id: int = Depends(require_user_id),
):
    aid = None
    if payload:
        aid = payload.artist_id or payload.id or payload.artistId
    if not aid:
        aid = artist_id or id
    aid = (aid or "").strip()
    if not aid:
        raise HTTPException(status_code=400, detail="artist_id is required")

    col = db_handler.get_collection("user_favourite_artists").collection
    res = await col.update_one(
        {"user_id": int(user_id), "artist_id": aid},
        {
            "$setOnInsert": {"created_at": time.time()},
            "$set": {"user_id": int(user_id), "artist_id": aid, "updated_at": time.time()},
        },
        upsert=True,
    )
    if res.upserted_id is not None:
        try:
            await db_handler.get_collection("artists").collection.update_one(
                {"_id": aid},
                {"$inc": {"followers": 1}}
            )
        except Exception:
            pass
    return {"ok": True, "already_exists": res.upserted_id is None}


@router.delete("/artists/favourites/{artist_id}")
@router.delete("/artist/favorite/{artist_id}")
@router.delete("/artists/favorite/{artist_id}")
async def remove_artist_favourite(artist_id: str, user_id: int = Depends(require_user_id)):
    aid = (artist_id or "").strip()
    if not aid:
        raise HTTPException(status_code=400, detail="artist_id is required")

    col = db_handler.get_collection("user_favourite_artists").collection
    res = await col.delete_one({"user_id": int(user_id), "artist_id": aid})
    if getattr(res, "deleted_count", 0) > 0:
        try:
            await db_handler.get_collection("artists").collection.update_one(
                {"_id": aid},
                {"$inc": {"followers": -1}}
            )
        except Exception:
            pass
    return {"ok": True, "deleted": bool(getattr(res, "deleted_count", 0))}


@router.get("/artists/favourites/ids")
async def list_favourite_artist_ids(user_id: Optional[int] = Depends(get_optional_user_id)):
    if user_id is None:
        return {"ok": True, "ids": []}

    col = db_handler.get_collection("user_favourite_artists").collection
    cursor = col.find({"user_id": int(user_id)}, {"_id": 0, "artist_id": 1})
    ids = []
    async for doc in cursor:
        aid = doc.get("artist_id")
        if aid:
            ids.append(aid)
    return {"ok": True, "ids": ids}


class ListeningEventItem(BaseModel):
    id: str
    track_id: str
    played_at: float
    started_at: float
    played_ms: float
    duration_ms: float
    completed: bool
    skipped: bool
    source: str = "server"
    session_id: Optional[str] = None


class ListeningEventsPayload(BaseModel):
    events: list[ListeningEventItem]


@router.post("/listening-events")
async def record_listening_events(
    payload: ListeningEventsPayload,
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    if not payload.events:
        return {"ok": True, "count": 0}

    col = db_handler.get_collection("listening_events").collection
    ops = []
    for ev in payload.events:
        eid = (ev.id or "").strip()
        if not eid:
            continue
        ops.append(
            UpdateOne(
                {"event_id": eid, "user_id": int(user_id) if user_id else None},
                {
                    "$setOnInsert": {"created_at": time.time()},
                    "$set": {
                        "event_id": eid,
                        "user_id": int(user_id) if user_id else None,
                        "track_id": ev.track_id,
                        "played_at": ev.played_at,
                        "started_at": ev.started_at,
                        "played_ms": ev.played_ms,
                        "duration_ms": ev.duration_ms,
                        "completed": ev.completed,
                        "skipped": ev.skipped,
                        "source": ev.source,
                        "session_id": ev.session_id,
                    },
                },
                upsert=True,
            )
        )
    if ops:
        await col.bulk_write(ops, ordered=False)
    return {"ok": True, "count": len(ops)}

