from pydantic import BaseModel


class TgLoginRequest(BaseModel):
    init_data: str
    username: str | None = None
    password: str | None = None


class PasswordLoginRequest(BaseModel):
    username: str
    password: str


class SetCredentialsRequest(BaseModel):
    username: str
    password: str


class SetCookieRequest(BaseModel):
    token: str


class OwnerPasswordLoginRequest(BaseModel):
    password: str


class SetOwnerPasswordRequest(BaseModel):
    password: str


class ChangeOwnerPasswordRequest(BaseModel):
    password: str


class FCMTokenRequest(BaseModel):
    fcm_token: str

class RegisterRequest(BaseModel):
    userid: int
    username: str
    password: str
    invite_code: str | None = None

class ValidateOTPRequest(BaseModel):
    userid: int
    otp: str


class TelegramWidgetLoginRequest(BaseModel):
    model_config = {"extra": "allow"}

    id: int
    first_name: str | None = None
    last_name: str | None = None
    username: str | None = None
    photo_url: str | None = None
    auth_date: int
    hash: str
    invite_code: str | None = None


class TelegramTokenLoginRequest(BaseModel):
    id_token: str
    invite_code: str | None = None


class DiscordIntegrationSchema(BaseModel):
    model_config = {"extra": "allow"}
    enabled: bool = False
    token: str = ""
    mode: str = "gateway"
    client_id: str = "1547543416143876167"
    daemon_url: str = "ws://127.0.0.1:6472"
    show_artwork: bool = True


class LastfmIntegrationSchema(BaseModel):
    model_config = {"extra": "allow"}
    enabled: bool = False
    api_key: str = ""
    api_secret: str = ""
    session_key: str = ""
    username: str = ""
    scrobble_at: float = 0.5
    now_playing: bool = True


class IntegrationsUpdateRequest(BaseModel):
    discord: DiscordIntegrationSchema | None = None
    lastfm: LastfmIntegrationSchema | None = None

