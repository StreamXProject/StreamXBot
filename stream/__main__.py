import asyncio
import os
import signal
import sys

if os.name != "nt":
    try:
        import uvloop

        uvloop.install()
    except Exception:
        pass

from uvicorn import Config as UvicornConfig
from uvicorn import Server as UvicornServer

from stream.core.config_manager import Config
from stream.plugins.dev.updater import restart_notification

from . import (
    add_daily_playlist_jobs,
    add_deleted_track_reconcile_jobs,
    add_user_profile_refresh_jobs,
    bot,
    scheduler,
)
from .database.MongoDb import db_handler
from .helpers.logger import LOGGER


def get_api_port() -> int:
    if port := os.environ.get("PORT"):
        try:
            return int(port)
        except ValueError:
            pass
    try:
        return int(getattr(Config, "API_PORT", 8000))
    except Exception:
        return 8000


async def start_api(log):
    try:
        from Api.main import app as fastapi_app

        cfg = UvicornConfig(
            app=fastapi_app,
            host="0.0.0.0",
            port=get_api_port(),
            loop="asyncio",
            log_level="info",
        )

        server = UvicornServer(cfg)
        task = asyncio.create_task(server.serve())

        log.info(f"FastAPI started on http://0.0.0.0:{cfg.port}")
        return server, task

    except Exception as e:
        log.warning(f"FastAPI failed to start: {e}")
        return None, None


async def cancel_pyrogram_pending_tasks(*, timeout: float = 2.0) -> None:
    tasks: list[asyncio.Task] = []
    for t in asyncio.all_tasks():
        if t is asyncio.current_task() or t.done():
            continue
        coro = t.get_coro()
        module = getattr(coro, "__module__", "") or ""
        if module.startswith("pyrogram.") or module.startswith("stream."):
            tasks.append(t)

    if not tasks:
        return

    for t in tasks:
        t.cancel()

    try:
        await asyncio.wait_for(
            asyncio.gather(*tasks, return_exceptions=True), timeout=timeout
        )
    except (asyncio.TimeoutError, asyncio.CancelledError):
        pass


async def close_aiohttp_sessions() -> None:
    try:
        from Api.services.stream_service import close_stream_hubs

        await asyncio.wait_for(close_stream_hubs(), timeout=2.0)
    except Exception:
        pass


async def main():
    log = LOGGER(__name__)

    # Set up graceful shutdown event & signal handlers
    shutdown_event = asyncio.Event()
    loop = asyncio.get_running_loop()

    force_exit_count = 0

    def _on_signal(signum):
        nonlocal force_exit_count
        force_exit_count += 1
        if force_exit_count >= 2:
            print("\n[CRITICAL] Force exit requested. Terminating immediately...", flush=True)
            os._exit(1)
        print("\n[INFO] Stop signal received. Shutting down gracefully... (Press Ctrl+C again to force exit)", flush=True)
        shutdown_event.set()

    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, lambda s=sig: _on_signal(s))
        except (NotImplementedError, RuntimeError):
            signal.signal(sig, lambda signum, frame: _on_signal(signum))

    def _on_restart_signal():
        log.info("Restart signal (SIGHUP) received. Performing graceful restart...")
        from stream.plugins.dev.updater import graceful_restart
        asyncio.create_task(graceful_restart())

    if hasattr(signal, "SIGHUP"):
        try:
            loop.add_signal_handler(signal.SIGHUP, _on_restart_signal)
        except (NotImplementedError, RuntimeError):
            signal.signal(signal.SIGHUP, lambda *_: _on_restart_signal())

    server = None
    server_task = None
    userbot = None
    userbot_task = None
    bot_started = False
    multi_clients_started = False
    enrichment_started = False
    scheduler_started = False

    try:
        log.info("Initializing MongoDB...")
        await db_handler.initialize()

        await Config.load_from_db()

        only_api = bool(getattr(Config, "ONLY_API", False))

        if scheduler is not None:
            try:
                add_daily_playlist_jobs(log)
                add_user_profile_refresh_jobs(log)
                add_deleted_track_reconcile_jobs(log)
                if not bool(getattr(scheduler, "running", False)):
                    scheduler.start()
                    scheduler_started = True
            except Exception as e:
                log.warning(f"Scheduler failed to start: {e}")

        if only_api:
            server, server_task = await start_api(log)
            log.info("API-only mode enabled. Running until stopped.")
            await shutdown_event.wait()
            return

        if bot is None:
            raise SystemExit("bot is disabled but ONLY_API is False")

        from stream import initialize_multi_clients, stop_multi_clients
        from stream.plugins.db.audioIndex import (
            start_enrichment_workers,
            stop_enrichment_workers,
        )
        from stream.plugins.userBot import start_userbot_service, stop_userbot_service

        await bot.start()
        bot_started = True

        me = await bot.get_me()
        await initialize_multi_clients(log, primary_user_id=int(getattr(me, "id")))
        multi_clients_started = True

        try:
            await restart_notification()
        except Exception as e:
            log.warning(f"Restart notification failed: {e}")

        server, server_task = await start_api(log)

        log.info("Client started. Running until stopped.")
        userbot, userbot_task = await start_userbot_service(log)
        start_enrichment_workers()
        enrichment_started = True

        log.info(f"{me.first_name} (@{me.username}) [ID: {me.id}]")

        # Wait until stop signal received
        await shutdown_event.wait()

    finally:
        log.info("Shutting down...")

        if enrichment_started:
            try:
                from stream.plugins.db.audioIndex import stop_enrichment_workers

                await asyncio.wait_for(stop_enrichment_workers(timeout=2.0), timeout=2.5)
            except Exception as e:
                log.debug(f"stop_enrichment_workers error: {e}")

        if server:
            server.should_exit = True
            server.force_exit = True
            if server_task:
                try:
                    server_task.cancel()
                    await asyncio.wait_for(server_task, timeout=2.0)
                except (asyncio.CancelledError, asyncio.TimeoutError):
                    pass
                except Exception as e:
                    log.debug(f"server_task error: {e}")

        if userbot or userbot_task:
            try:
                from stream.plugins.userBot import stop_userbot_service

                await asyncio.wait_for(
                    stop_userbot_service(userbot, userbot_task), timeout=2.5
                )
            except Exception as e:
                log.debug(f"stop_userbot_service error: {e}")

        await close_aiohttp_sessions()

        if scheduler is not None and (scheduler_started or getattr(scheduler, "running", False)):
            try:
                scheduler.shutdown(wait=False)
            except Exception:
                pass

        if multi_clients_started:
            try:
                from stream import stop_multi_clients

                await asyncio.wait_for(stop_multi_clients(log), timeout=2.5)
            except Exception as e:
                log.debug(f"stop_multi_clients error: {e}")

        if bot_started and bot is not None:
            try:
                await asyncio.wait_for(bot.stop(), timeout=2.5)
            except Exception as e:
                log.debug(f"bot.stop error: {e}")

        try:
            await asyncio.wait_for(db_handler.close(), timeout=2.0)
        except Exception:
            pass

        await cancel_pyrogram_pending_tasks(timeout=2.0)
        log.info("Client stopped.")


if __name__ == "__main__":
    exit_code = 0
    try:
        if bot is not None and getattr(bot, "loop", None) is not None:
            bot.loop.run_until_complete(main())
        else:
            asyncio.run(main())
    except KeyboardInterrupt:
        pass
    except SystemExit as e:
        exit_code = e.code if isinstance(e.code, int) else (1 if e.code else 0)
    except Exception as e:
        LOGGER(__name__).error(f"Fatal error: {e}", exc_info=True)
        exit_code = 1
    finally:
        os._exit(exit_code)
