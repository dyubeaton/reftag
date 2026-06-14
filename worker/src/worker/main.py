import os
import sys
import time

import httpx
import redis


def get_env(name: str) -> str:
    """Read a required environment variable or exit loudly."""
    value = os.getenv(name)
    if not value:
        print(f"FATAL: {name} not set", file=sys.stderr)
        sys.exit(1)
    return value


def main() -> None:
    redis_addr = get_env("REDIS_ADDR")
    backend_url = get_env("BACKEND_URL")

    # --- Connect to Redis ---
    host, port = redis_addr.split(":")
    rdb = redis.Redis(host=host, port=int(port), decode_responses=True) #decode pong from bytes to str

    # Retry loop beyond the first connection in case a dependency goes down (don't assume dependencies eternally present)
    for attempt in range(1, 11):
        try:
            rdb.ping()
            print("connected to redis", flush=True) # set flush to force each line out immediately 
            break
        except redis.ConnectionError:
            print(f"redis not ready (attempt {attempt}/10), retrying...", flush=True)
            time.sleep(2)
    else: 
        print("FATAL: could not reach redis", file=sys.stderr) #runs if the loop completes without hitting break
        sys.exit(1)

    # --- Confirm the Go API is reachable ---
    try:
        resp = httpx.get(f"{backend_url}/health", timeout=5.0)
        print(f"backend /health responded: {resp.status_code}", flush=True)
    except httpx.HTTPError as e:
        print(f"WARNING: could not reach backend: {e}", flush=True) # soft dependency

    # --- Idle loop (real job processing comes next step) ---
    print("worker started, waiting for jobs...", flush=True)
    while True:
        time.sleep(5)
        print("worker heartbeat (no jobs yet)", flush=True)


if __name__ == "__main__":
    main()