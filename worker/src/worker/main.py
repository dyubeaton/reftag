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

    # --- Ensure our consumer group exists on the jobs stream ---
    try:
        rdb.xgroup_create(name="jobs", groupname="workers", id="$", mkstream=True)
        print("created jobs consumer group", flush=True)
    except redis.ResponseError as e:
        if "BUSYGROUP" in str(e):
            print("jobs consumer group already exists", flush=True)
        else:
            raise

    print("worker started, waiting for jobs...", flush=True)

    consumer_name = "worker-1"
    while True:
        # Block up to 5s waiting for undelivered jobs (">").
        resp = rdb.xreadgroup(
            groupname="workers",
            consumername=consumer_name,
            streams={"jobs": ">"},
            count=10,
            block=5000,  # milliseconds
        )

        if not resp:
            continue  # timed out with no new jobs — normal

        for stream_name, messages in resp:
            for msg_id, fields in messages:
                print(f"processing job {msg_id}: {fields}", flush=True)

                # --- Dummy "work" ---
                # Real version: query Go API for tags, run CLIP, etc.
                result = {
                    "job_id": msg_id,
                    "image_id": fields.get("image_id", "unknown"),
                    "suggested_tags": "hand,pose",  # hardcoded for the skeleton
                }

                # Produce a result onto the results stream.
                rdb.xadd("results", result)
                print(f"produced result for job {msg_id}", flush=True)

                # Acknowledge the original job.
                rdb.xack("jobs", "workers", msg_id)


if __name__ == "__main__":
    main()