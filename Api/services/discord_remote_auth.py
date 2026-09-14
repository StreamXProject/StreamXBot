"""
Discord Remote Auth (QR Code Login) Service.

Implements the Discord Remote Auth Gateway protocol (v2) over WebSocket:
- Connects to wss://remote-auth-gateway.discord.gg/?v=2 with Origin: https://discord.com
- Generates an ephemeral RSA-2048 keypair
- Handles the hello -> init -> nonce_proof challenge
- Produces the QR code fingerprint URL (https://discord.com/ra/<fingerprint>)
- Awaits mobile scan (pending_remote_init) & approval (pending_ticket)
- Exchanges ticket for the user token via https://discord.com/api/v9/users/@me/remote-auth/login
"""
from __future__ import annotations

import asyncio
import base64
import hashlib
import json
import time
from typing import AsyncGenerator, Dict, Any, Optional

import aiohttp
from cryptography.hazmat.primitives.asymmetric import rsa, padding
from cryptography.hazmat.primitives import hashes, serialization

from stream.helpers.logger import LOGGER

GATEWAY_URL = "wss://remote-auth-gateway.discord.gg/?v=2"
LOGIN_API_URL = "https://discord.com/api/v9/users/@me/remote-auth/login"

HEADERS = {
    "Origin": "https://discord.com",
    "User-Agent": (
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
    ),
}


class DiscordRemoteAuthSession:
    def __init__(self, session_id: str) -> None:
        self.session_id = session_id
        self._privkey = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        self._cancelled = False
        self._ws: Optional[aiohttp.ClientWebSocketResponse] = None
        self._heartbeat_task: Optional[asyncio.Task] = None

        spki_der = self._privkey.public_key().public_bytes(
            encoding=serialization.Encoding.DER,
            format=serialization.PublicFormat.SubjectPublicKeyInfo,
        )
        self.public_key_b64 = base64.b64encode(spki_der).decode("utf-8")

        # Session state
        self.status = "init"
        self.fingerprint: Optional[str] = None
        self.qr_url: Optional[str] = None
        self.user_info: Optional[Dict[str, Any]] = None
        self.token: Optional[str] = None
        self.error: Optional[str] = None
        self.created_at = time.time()

    def cancel(self) -> None:
        self._cancelled = True
        self.status = "cancelled"
        if self._heartbeat_task and not self._heartbeat_task.done():
            self._heartbeat_task.cancel()
        if self._ws and not self._ws.closed:
            asyncio.create_task(self._ws.close())

    def _decrypt_oaep(self, enc_b64: str) -> bytes:
        cipher_bytes = base64.b64decode(enc_b64)
        return self._privkey.decrypt(
            cipher_bytes,
            padding.OAEP(
                mgf=padding.MGF1(algorithm=hashes.SHA256()),
                algorithm=hashes.SHA256(),
                label=None,
            ),
        )

    async def _heartbeat_loop(self, interval_ms: int) -> None:
        interval_sec = max(5.0, (interval_ms / 1000.0) * 0.9)
        try:
            while not self._cancelled and self._ws and not self._ws.closed:
                await asyncio.sleep(interval_sec)
                if self._ws and not self._ws.closed:
                    await self._ws.send_json({"op": "heartbeat"})
        except asyncio.CancelledError:
            pass
        except Exception as exc:
            LOGGER(__name__).debug(f"[DiscordRA {self.session_id}] Heartbeat ended: {exc}")

    async def stream_events(self) -> AsyncGenerator[Dict[str, Any], None]:
        """Stream real-time lifecycle events to the caller."""
        yield {"type": "status", "status": "connecting", "session_id": self.session_id}

        connector = aiohttp.TCPConnector(ssl=True)
        timeout = aiohttp.ClientTimeout(total=180)

        async with aiohttp.ClientSession(headers=HEADERS, connector=connector, timeout=timeout) as http:
            try:
                async with http.ws_connect(GATEWAY_URL) as ws:
                    self._ws = ws
                    LOGGER(__name__).info(f"[DiscordRA {self.session_id}] Connected to Remote Auth Gateway")

                    async for msg in ws:
                        if self._cancelled:
                            break

                        if msg.type == aiohttp.WSMsgType.TEXT:
                            try:
                                payload = json.loads(msg.data)
                            except json.JSONDecodeError:
                                continue

                            op = payload.get("op")

                            if op == "hello":
                                heartbeat_interval = payload.get("heartbeat_interval", 41250)
                                self._heartbeat_task = asyncio.create_task(
                                    self._heartbeat_loop(heartbeat_interval)
                                )
                                # Send public key in init
                                await ws.send_json({
                                    "op": "init",
                                    "encoded_public_key": self.public_key_b64,
                                })

                            elif op == "nonce_proof":
                                enc_nonce = payload.get("encrypted_nonce", "")
                                try:
                                    decrypted_nonce = self._decrypt_oaep(enc_nonce)
                                    digest = hashlib.sha256(decrypted_nonce).digest()
                                    proof = (
                                        base64.urlsafe_b64encode(digest)
                                        .decode("utf-8")
                                        .rstrip("=")
                                    )
                                    await ws.send_json({
                                        "op": "nonce_proof",
                                        "proof": proof,
                                    })
                                except Exception as exc:
                                    self.error = f"Nonce proof failed: {exc}"
                                    self.status = "error"
                                    yield {"type": "error", "message": self.error}
                                    return

                            elif op in ("fingerprint", "pending_remote_init"):
                                # If payload has a fingerprint and no user yet, this is the QR initialization
                                fp = payload.get("fingerprint")
                                user = payload.get("user")

                                if fp and not self.fingerprint:
                                    self.fingerprint = fp
                                    self.qr_url = f"https://discord.com/ra/{fp}"
                                    self.status = "waiting_scan"
                                    yield {
                                        "type": "qr",
                                        "url": self.qr_url,
                                        "fingerprint": self.fingerprint,
                                        "session_id": self.session_id,
                                    }

                                if user:
                                    # User has scanned the QR code with their mobile Discord app!
                                    self.user_info = {
                                        "id": user.get("id"),
                                        "username": user.get("username"),
                                        "discriminator": user.get("discriminator"),
                                        "avatar": user.get("avatar"),
                                    }
                                    self.status = "scanned"
                                    yield {
                                        "type": "scanned",
                                        "user": self.user_info,
                                        "session_id": self.session_id,
                                    }

                            elif op == "pending_ticket":
                                # Mobile scanned the code — decrypts user info from encrypted_user_payload
                                enc_payload = payload.get("encrypted_user_payload")
                                if enc_payload:
                                    try:
                                        decrypted_bytes = self._decrypt_oaep(enc_payload)
                                        decrypted_text = decrypted_bytes.decode("utf-8")
                                        # Format: user_id:discriminator:avatar_hash:username
                                        parts = decrypted_text.split(":")
                                        u_id = parts[0] if len(parts) > 0 else None
                                        u_disc = parts[1] if len(parts) > 1 else "0"
                                        u_av = parts[2] if len(parts) > 2 and parts[2] not in ("None", "") else None
                                        u_name = parts[3] if len(parts) > 3 else "Discord User"
                                        self.user_info = {
                                            "id": u_id,
                                            "username": u_name,
                                            "discriminator": u_disc,
                                            "avatar": u_av,
                                        }
                                        LOGGER(__name__).info(f"[DiscordRA {self.session_id}] Mobile scan confirmed for @{u_name}")
                                    except Exception as exc:
                                        LOGGER(__name__).warning(f"[DiscordRA {self.session_id}] Failed to parse user payload: {exc}")

                                self.status = "scanned"
                                yield {
                                    "type": "scanned",
                                    "user": self.user_info,
                                    "session_id": self.session_id,
                                }

                            elif op == "pending_login":
                                # Mobile tapped "Log in" — payload contains the actual authorization ticket!
                                ticket = payload.get("ticket")
                                if not ticket:
                                    LOGGER(__name__).warning(f"[DiscordRA {self.session_id}] pending_login without ticket: {payload}")
                                    continue

                                self.status = "confirming"
                                yield {"type": "status", "status": "confirming", "session_id": self.session_id}

                                # Exchange ticket for token via Discord REST API
                                try:
                                    async with http.post(
                                        LOGIN_API_URL,
                                        json={"ticket": ticket},
                                        headers={"Content-Type": "application/json"},
                                    ) as resp:
                                        if resp.status != 200:
                                            body = await resp.text()
                                            self.error = f"Ticket exchange failed ({resp.status}): {body}"
                                            self.status = "error"
                                            yield {"type": "error", "message": self.error}
                                            return

                                        login_data = await resp.json()
                                        enc_token = login_data.get("encrypted_token")
                                        if not enc_token:
                                            self.error = "No encrypted_token in Discord response"
                                            self.status = "error"
                                            yield {"type": "error", "message": self.error}
                                            return

                                        token_bytes = self._decrypt_oaep(enc_token)
                                        self.token = token_bytes.decode("utf-8")
                                        self.status = "success"

                                        LOGGER(__name__).info(f"[DiscordRA {self.session_id}] Successfully extracted Discord token for user!")

                                        yield {
                                            "type": "success",
                                            "token": self.token,
                                            "user": self.user_info,
                                            "session_id": self.session_id,
                                        }

                                        # Handshake complete — clean exit
                                        try:
                                            await ws.send_json({"op": "cancel"})
                                        except Exception:
                                            pass
                                        return
                                except Exception as exc:
                                    self.error = f"Error during token exchange: {exc}"
                                    self.status = "error"
                                    yield {"type": "error", "message": self.error}
                                    return

                            elif op == "cancel":
                                self.status = "cancelled"
                                yield {"type": "cancelled", "session_id": self.session_id}
                                return

                        elif msg.type in (aiohttp.WSMsgType.CLOSED, aiohttp.WSMsgType.ERROR):
                            break

            except Exception as exc:
                if not self._cancelled:
                    LOGGER(__name__).error(f"[DiscordRA {self.session_id}] WebSocket error: {exc}")
                    self.error = str(exc)
                    self.status = "error"
                    yield {"type": "error", "message": f"Connection error: {exc}"}
            finally:
                if self._heartbeat_task and not self._heartbeat_task.done():
                    self._heartbeat_task.cancel()
                self._ws = None
