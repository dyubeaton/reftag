# RefTag

A local reference image library for artists. Import images, tag them, search by tag combinations.

See [DESIGN.md](./DESIGN.md) for architecture and decisions.

## Status

Walking skeleton in progress. Not yet runnable.

## Stack

- **Backend:** Go (chi router, pgx, go-redis)
- **Worker:** Python (open_clip, redis-py)
- **Database:** Postgres
- **Message broker:** Redis (Streams)
- **Orchestration:** Docker Compose

## Development

Requires Docker Desktop, Go 1.22+, Python 3.11+.

```bash
docker compose up
```

(Coming soon — services don't start yet.)