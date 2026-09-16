from typing import Any

from pydantic import BaseModel, ConfigDict, Field

class TrackResponse(BaseModel):
    model_config = ConfigDict(populate_by_name=True, ser_json_by_alias=True)

    id: str = Field(alias="_id")
    source_chat_id: int | None = None
    source_message_id: int | None = None
    telegram: dict[str, Any] | None = None
    audio: dict[str, Any] | None = None
    spotify: dict[str, Any] | None = None
    content_hash: str | None = None
    fingerprint: str | None = None
    created_at: float | None = None
    updated_at: float | None = None
    liked: bool = False

