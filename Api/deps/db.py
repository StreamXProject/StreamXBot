from stream.database.MongoDb import db_handler

async def init_db():
    await db_handler.initialize()
    try:
        ft_docs = await db_handler.get_collection("forum_topics").collection.find(
            {"topic_name": {"$exists": True, "$not": {"$regex": r"^topic_"}}}
        ).to_list(100)
        for ft in ft_docs:
            t_id = ft.get("topic_id")
            t_name = ft.get("topic_name")
            if t_id and t_name:
                await db_handler.audio_collection.collection.update_many(
                    {"topic_id": int(t_id), "topic_name": {"$regex": r"^topic_"}},
                    {"$set": {"topic_name": t_name}},
                )
    except Exception:
        pass

def get_audio_tracks_collection():
    return db_handler.audio_collection.collection

