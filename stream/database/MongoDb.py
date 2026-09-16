import sys
import time
from typing import Union, Optional, Dict, List

from pymongo import AsyncMongoClient
from stream.helpers.logger import LOGGER
from stream.core.config_manager import Config
from config import MONGO_URI

DEFAULT_DB = "Stream"

class MongoDB:
    def __init__(self, collection):
        self.collection = collection

    async def read_document(self, document_id: Union[str, int], projection: Optional[Dict] = None) -> Optional[Dict]:
        try:
            if projection:
                return await self.collection.find_one({"_id": document_id}, projection)
            return await self.collection.find_one({"_id": document_id})
        except Exception as e:
            LOGGER(__name__).warning(f"MongoDB.read_document failed on '{self.collection.name}' id={document_id}: {e}")
            raise

    async def find_document(self, filter: Dict, projection: Optional[Dict] = None) -> Optional[Dict]:
        try:
            if projection:
                return await self.collection.find_one(filter, projection)
            return await self.collection.find_one(filter)
        except Exception as e:
            LOGGER(__name__).warning(f"MongoDB.find_document failed on '{self.collection.name}' filter={filter}: {e}")
            raise

    async def update_document(self, document_id: Union[str, int], updated_data: Dict) -> None:
        try:
            await self.collection.update_one({"_id": document_id}, {"$set": updated_data}, upsert=True)
        except Exception as e:
            LOGGER(__name__).error(f"MongoDB.update_document failed on '{self.collection.name}' id={document_id} data={updated_data}: {e}")
            raise

    async def delete_document(self, document_id: Union[str, int]) -> bool:
        try:
            result = await self.collection.delete_one({"_id": document_id})
            return result.deleted_count > 0
        except Exception as e:
            LOGGER(__name__).warning(f"MongoDB.delete_document failed on '{self.collection.name}' id={document_id}: {e}")
            raise

    async def total_documents(self) -> int:
        try:
            return await self.collection.count_documents({})
        except Exception as e:
            LOGGER(__name__).warning(f"MongoDB.total_documents failed on '{self.collection.name}': {e}")
            raise

    async def get_all_id(self) -> List[Union[str, int]]:
        try:
            return await self.collection.distinct("_id")
        except Exception as e:
            LOGGER(__name__).warning(f"MongoDB.get_all_id failed on '{self.collection.name}': {e}")
            raise

    async def update_one(self, filter: Dict, update: Dict, upsert: bool = False):
        try:
            return await self.collection.update_one(filter, update, upsert=upsert)
        except Exception as e:
            LOGGER(__name__).warning(f"MongoDB.update_one failed on '{self.collection.name}' filter={filter} update={update} upsert={upsert}: {e}")
            raise

    async def find_all(self, filter: Optional[Dict] = None, projection: Optional[Dict] = None) -> "AsyncIterator[Dict]":
        try:
            if filter is None:
                filter = {}
            cursor = self.collection.find(filter, projection)
            async for document in cursor:
                yield document
        except Exception as e:
            LOGGER(__name__).warning(f"MongoDB.find_all failed on '{self.collection.name}' filter={filter}: {e}")
            raise


class MongoDatabase:
    """
    Manages connection and collection wrappers using PyMongo's async client.
    """
    def __init__(self):
        self.client: Optional[AsyncMongoClient] = None
        self.database = None
        self._collections: Dict[str, MongoDB] = {}

    async def initialize(self, mongo_uri: Optional[str] = None, db_name: Optional[str] = None):
        try:
            mongo_uri = mongo_uri or (Config.MONGO_URI or MONGO_URI)
            db_name = db_name or (Config.DATABASE_NAME or DEFAULT_DB)
            self.client = AsyncMongoClient(mongo_uri)

            await self.client.admin.command("ping")

            existing = await self.client.list_database_names()
            if db_name not in existing:
                LOGGER(__name__).info(f"Database '{db_name}' not found. Initializing...")
                marker_col = self.client[db_name]["__init"]
                await marker_col.insert_one({"created_at": time.time()})
                LOGGER(__name__).info(f"Database '{db_name}' initialized with marker collection")
            else:
                LOGGER(__name__).info(f"Using existing database '{db_name}'")

            self.database = self.client[db_name]

        except Exception as error:
            LOGGER(__name__).error(f"MongoDB connection failed: {error}")
            sys.exit(1)

        self._initialize_collections()
        await self._ensure_indexes()

    async def close(self):
        client = self.client
        self.client = None
        self.database = None
        self._collections = {}
        if not client:
            return
        try:
            aclose = getattr(client, "aclose", None)
            if callable(aclose):
                await aclose()
                return
            close = getattr(client, "close", None)
            if callable(close):
                close()
        except Exception as e:
            LOGGER(__name__).warning(f"MongoDB close failed: {e}")

    def _initialize_collections(self):
        collections: Dict[str, str] = {
            'users': 'users',
            'chats_collection': 'chats',
            'channels_collection': 'channels',
            'botsettings': 'botsettings',
            'audio_collection': 'audioTracks',
            'jam_sessions': 'jamSessions',
            'user_playlists': 'userPlaylists',
            'playlist_tracks': 'playlistTracks',
            'user_favourites': 'userFavourites',
            'user_history': 'userHistory',
            'userplayback_collection': 'userPlayback',
            'globalplayback_collection': 'globalPlayback',
            'friends_collection': 'friends',
            'friend_requests_collection': 'friendRequests',
            'presence_collection': 'presence',
            'listening_status_collection': 'listeningStatus',
            'notifications_collection': 'notifications',
            'allowed_sources': 'allowedSources',
            'banned_sources': 'bannedSources',
        }
        for attr, col_name in collections.items():
            coll = self.database[col_name]
            wrapper = MongoDB(coll)
            setattr(self, attr, wrapper)
            self._collections[attr] = wrapper
            self._collections[col_name] = wrapper

    async def _ensure_indexes(self) -> None:
        try:
            user_col = self.userplayback_collection.collection
            await user_col.create_index([("user_id", 1), ("track_id", 1), ("bucket", 1)], unique=True)
            await user_col.create_index([("user_id", 1), ("played_at", -1)])
            await user_col.create_index([("track_id", 1), ("played_at", -1)])
        except Exception:
            pass
        try:
            global_col = self.globalplayback_collection.collection
            await global_col.create_index([("plays", -1)])
            await global_col.create_index([("last_played_at", -1)])
        except Exception:
            pass
        try:
            presence_col = self.presence_collection.collection
            await presence_col.create_index([("user_id", 1)], unique=True)
            await presence_col.create_index([("last_seen", 1)], expireAfterSeconds=90)
        except Exception:
            pass
        try:
            listening_col = self.listening_status_collection.collection
            await listening_col.create_index([("user_id", 1)], unique=True)
        except Exception:
            pass
        try:
            notif_col = self.notifications_collection.collection
            await notif_col.create_index([("user_id", 1), ("created_at", -1)])
        except Exception:
            pass
        try:
            freq_col = self.friend_requests_collection.collection
            await freq_col.create_index([("to", 1), ("from", 1), ("status", 1)])
        except Exception:
            pass
        try:
            audio_col = self.audio_collection.collection
            await audio_col.create_index([("deleted", 1), ("audio.album", 1), ("audio.artist", 1)])
        except Exception:
            pass
        try:
            audio_col = self.audio_collection.collection
            await audio_col.create_index([("deleted", 1), ("audio.artist", 1)])
            await audio_col.create_index([("deleted", 1), ("audio.performer", 1)])
        except Exception:
            pass
        try:
            audio_col = self.audio_collection.collection
            await audio_col.create_index([("deleted", 1), ("audio.artists", 1)])
        except Exception:
            pass
        try:
            audio_col = self.audio_collection.collection
            await audio_col.create_index([("deleted", 1), ("audio.album_id", 1)])
        except Exception:
            pass
        try:
            audio_col = self.audio_collection.collection
            await audio_col.create_index([("content_hash", 1)], unique=True, sparse=True)
            await audio_col.create_index([("fingerprint", 1)])
            await audio_col.create_index([("deleted", 1), ("topic_name", 1), ("created_at", -1), ("source_message_id", -1)], name="idx_topic_browse_created")
            await audio_col.create_index([("deleted", 1), ("source_chat_id", 1), ("topic_name", 1), ("created_at", -1)], name="idx_chat_topic_browse_created")
            await audio_col.create_index([("created_at", -1)], name="idx_created_at")
            await audio_col.create_index([("deleted", 1), ("topic_name", 1), ("updated_at", -1), ("source_message_id", -1)], name="idx_topic_browse")
            await audio_col.create_index([("deleted", 1), ("source_chat_id", 1), ("topic_name", 1), ("updated_at", -1)], name="idx_chat_topic_browse")
            await audio_col.create_index([("topic_name", 1)], name="idx_topic_name")
            await audio_col.create_index([("topic_id", 1)], name="idx_topic_id")
            await audio_col.create_index([("enriched", 1), ("deleted", 1), ("enriching", 1), ("enrich_retry_after", 1), ("source_message_id", -1)], name="idx_enrichment_queue")
            await audio_col.create_index([("enriched", 1), ("deleted", 1)], name="idx_enriched_status")
            try:
                await audio_col.update_many(
                    {"created_at": {"$exists": False}},
                    [{"$set": {"created_at": {"$ifNull": ["$updated_at", "$enriched_at", time.time()]}}}],
                )
            except Exception:
                pass
        except Exception:
            pass
        try:
            albums_col = self.get_collection("albums").collection
            await albums_col.create_index([("updated_at", -1)])
            await albums_col.create_index([("artist", 1)])
            await albums_col.create_index([("match_artist", 1)])
        except Exception:
            pass
        try:
            artists_col = self.get_collection("artists").collection
            await artists_col.create_index([("updated_at", -1)])
            await artists_col.create_index([("match_artist", 1)])
        except Exception:
            pass
        try:
            user_albums_col = self.get_collection("user_albums").collection
            await user_albums_col.create_index([("user_id", 1), ("album_id", 1)], unique=True)
            await user_albums_col.create_index([("user_id", 1), ("saved_at", -1)])
        except Exception:
            pass
        try:
            allowed_col = self.allowed_sources.collection
            await allowed_col.create_index([("source_id", 1)], unique=True)
            await allowed_col.create_index([("added_at", -1)])
        except Exception:
            pass
        try:
            banned_col = self.banned_sources.collection
            await banned_col.create_index([("source_id", 1)], unique=True)
            await banned_col.create_index([("banned_at", -1)])
        except Exception:
            pass

    def get_collection(self, col_name: str) -> MongoDB:
        key = (col_name or "").strip()
        if not key:
            raise ValueError("col_name is required")

        if key in self._collections:
            return self._collections[key]

        if self.database is not None:
            coll = self.database[key]
            wrapper = MongoDB(coll)
            self._collections[key] = wrapper
            return wrapper
        raise RuntimeError("Database not initialized")

    def __getattr__(self, name: str):
        if name in self._collections:
            return self._collections[name]
        if self.database is not None:
            collections: Dict[str, str] = {
                'users': 'users',
                'chats_collection': 'chats',
                'channels_collection': 'channels',
                'botsettings': 'botsettings',
                'audio_collection': 'audioTracks',
                'jam_sessions': 'jamSessions',
                'user_playlists': 'userPlaylists',
                'playlist_tracks': 'playlistTracks',
                'user_favourites': 'userFavourites',
                'user_history': 'userHistory',
                'userplayback_collection': 'userPlayback',
                'globalplayback_collection': 'globalPlayback',
                'friends_collection': 'friends',
                'friend_requests_collection': 'friendRequests',
                'presence_collection': 'presence',
                'listening_status_collection': 'listeningStatus',
                'notifications_collection': 'notifications',
                'allowed_sources': 'allowedSources',
                'banned_sources': 'bannedSources',
            }
            if name in collections:
                coll = self.database[collections[name]]
                wrapper = MongoDB(coll)
                setattr(self, name, wrapper)
                self._collections[name] = wrapper
                return wrapper
        raise AttributeError(f"No such collection wrapper: {name}")

db_handler = MongoDatabase()
