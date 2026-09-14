import logging
from logging.handlers import RotatingFileHandler

_level = logging.INFO
try:
    from config import DEBUG as _DEBUG

    if bool(_DEBUG):
        _level = logging.DEBUG
except Exception:
    pass

_datefmt = "%d-%b-%y %H:%M:%S"
_fmt_default = "[%(asctime)s - %(levelname)s] - %(name)s - %(message)s"
_fmt_ours = "[%(asctime)s - %(levelname)s] - %(name)s - %(filename)s:%(lineno)d - %(message)s"


class _ConditionalFormatter(logging.Formatter):
    def __init__(self):
        super().__init__(datefmt=_datefmt)
        self._default = logging.Formatter(_fmt_default, datefmt=_datefmt)
        self._ours = logging.Formatter(_fmt_ours, datefmt=_datefmt)

    def format(self, record: logging.LogRecord) -> str:
        if _level == logging.DEBUG and (record.name.startswith("stream") or record.name.startswith("Api")):
            return self._ours.format(record)
        return self._default.format(record)


root = logging.getLogger()
root.setLevel(_level)

if not root.handlers:
    file_handler = RotatingFileHandler("log.txt", mode="a", maxBytes=5_000_000, backupCount=3, encoding="utf-8")
    stream_handler = logging.StreamHandler()
    fmt = _ConditionalFormatter()
    file_handler.setFormatter(fmt)
    stream_handler.setFormatter(fmt)
    root.addHandler(file_handler)
    root.addHandler(stream_handler)
else:
    fmt = _ConditionalFormatter()
    for h in list(root.handlers):
        try:
            h.setFormatter(fmt)
        except Exception:
            pass

logging.getLogger("pymongo").setLevel(logging.WARNING)
logging.getLogger("asyncio").setLevel(logging.WARNING)
logging.getLogger("asyncio.windows_events").setLevel(logging.WARNING)
logging.getLogger("pyrogram").setLevel(logging.ERROR)
logging.getLogger("httpx").setLevel(logging.ERROR)
logging.getLogger("httpcore").setLevel(logging.WARNING)
logging.getLogger("httpcore.connection").setLevel(logging.WARNING)
logging.getLogger("httpcore.http11").setLevel(logging.WARNING)
logging.getLogger("apscheduler").setLevel(logging.WARNING)

def LOGGER(name: str) -> logging.Logger:
    return logging.getLogger(name) 
