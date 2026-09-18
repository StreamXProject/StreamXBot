import urllib.parse
from typing import Optional
from fastapi import APIRouter, Depends, HTTPException, Query

from Api.schemas.browse import BrowseResponse
from Api.schemas.topics import TopicsResponse
from Api.services.track_service import browse_topic_tracks, get_unique_topics
from Api.utils.auth import get_optional_user_id

router = APIRouter(tags=["topics"])


@router.get("/topics", response_model=TopicsResponse)
async def list_topics(
    channel_id: Optional[int] = None,
    limit: int = Query(default=100, ge=1, le=500),
    refresh: bool = Query(default=False),
):
    """
    Get all unique topic names with track counts, latest cover art, and endpoints.
    Optimized with MongoDB index-covered aggregation and in-memory TTL caching.
    """
    return await get_unique_topics(channel_id=channel_id, limit=limit, refresh=refresh)


@router.get("/topics/{topic_name:path}/tracks", response_model=BrowseResponse)
async def get_topic_tracks(
    topic_name: str,
    channel_id: Optional[int] = None,
    page: int = Query(default=1, ge=1),
    per_page: int = Query(default=20, ge=1, le=100),
    limit: Optional[int] = Query(default=None, ge=1, le=100),
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    """
    Browse tracks belonging to a unique topic_name.
    Optimized with compound index queries, lean projections, parallel count+find, and batch liked checks.
    """
    clean_topic = urllib.parse.unquote(topic_name or "").strip()
    if not clean_topic:
        raise HTTPException(status_code=400, detail="topic_name is required")

    effective_per_page = limit if limit is not None else per_page
    return await browse_topic_tracks(
        topic_name=clean_topic,
        channel_id=channel_id,
        page=page,
        per_page=effective_per_page,
        user_id=user_id,
    )
