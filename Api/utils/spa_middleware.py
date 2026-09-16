from __future__ import annotations

import os
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import FileResponse, Response


def is_html_navigation(request: Request) -> bool:
    if request.method not in ("GET", "HEAD"):
        return False

    sec_dest = (request.headers.get("sec-fetch-dest") or "").lower()
    if sec_dest in ("document", "frame", "iframe"):
        return True

    sec_mode = (request.headers.get("sec-fetch-mode") or "").lower()
    if sec_mode == "navigate":
        return True

    accept = (request.headers.get("accept") or "").lower()
    if "text/html" in accept:
        first_accept = accept.split(",")[0].strip()
        if not first_accept.startswith("application/json"):
            return True

    return False


class SPAMiddleware(BaseHTTPMiddleware):
    """
    Middleware that intercepts browser HTML page navigation requests
    (such as page reloads on /search, /albums, /artists, etc.)
    and serves the frontend SPA shell (index.html) instead of API JSON.

    API requests (e.g. from fetch/axios with Accept: application/json or non-HTML)
    and static assets/backend docs/auth popups pass through untouched.
    """

    def __init__(self, app, dist_dir: str):
        super().__init__(app)
        self.dist_dir = dist_dir

    async def dispatch(self, request: Request, call_next) -> Response:
        if not is_html_navigation(request):
            return await call_next(request)

        path = (request.url.path or "/").rstrip("/") or "/"

        if (
            path == "/docs"
            or path.startswith("/docs/")
            or path == "/redoc"
            or path.startswith("/redoc/")
            or path == "/openapi.json"
            or path.startswith("/assets/")
            or path.startswith("/auth/")
            or path.startswith("/covers/")
            or path == "/test-stream"
            or "/stream" in path
            or "/download" in path
            or "/warm" in path
        ):
            return await call_next(request)

        clean_path = path.lstrip("/")
        if clean_path:
            file_path = os.path.join(self.dist_dir, clean_path)
            if os.path.isfile(file_path):
                if clean_path in ("sw.js", "manifest.json"):
                    return FileResponse(file_path, headers={"Cache-Control": "no-cache, no-store, must-revalidate"})
                return FileResponse(file_path)

        index_file = os.path.join(self.dist_dir, "index.html")
        if os.path.isfile(index_file):
            return FileResponse(index_file, headers={"Cache-Control": "no-cache, no-store, must-revalidate"})

        return await call_next(request)
