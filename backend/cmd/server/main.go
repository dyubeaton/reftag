package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dyubeaton/reftag/backend/internal/queue"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// holds shared dependencies passed to handlers
type app struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func main() {
	ctx := context.Background()

	// Connect to Postgres
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("unable to create connection pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("unable to reach postgres: %v", err)
	}
	log.Println("connected to postgres")

	// Connect to Redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("REDIS_ADDR not set")
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("unable to reach redis: %v", err)
	}
	log.Println("connected to redis")

	if err := ensureGroup(ctx, rdb, queue.ResultsStream, queue.BackendGroup); err != nil {
		log.Fatalf("failed to create results consumer group: %v", err)
	}
	log.Println("results consumer group ready")

	// Wire up dependencies and routes
	a := &app{db: pool, redis: rdb}

	r := chi.NewRouter()

	// Runs in order w/every request
	r.Use(middleware.RequestID) // tag each request with unique ID
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer) //catches panics

	r.Get("/health", a.handleHealth)
	r.Post("/api/debug/enqueue", a.handleEnqueue)

	addr := ":8080"
	log.Printf("starting server on %s", addr)

	go a.consumeResults(ctx)
	log.Println("result consumer started")

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// handleHealth checks connectivity to Postgres and Redis and reports status.
func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	resp := map[string]string{
		"status":   "ok",
		"postgres": "ok",
		"redis":    "ok",
		"time":     time.Now().UTC().Format(time.RFC3339),
	}
	statusCode := http.StatusOK

	if err := a.db.Ping(ctx); err != nil {
		resp["postgres"] = "unreachable"
		resp["status"] = "degraded"
		statusCode = http.StatusServiceUnavailable
	}

	if err := a.redis.Ping(ctx).Err(); err != nil {
		resp["redis"] = "unreachable"
		resp["status"] = "degraded"
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

// ensureGroup creates a consumer group on a stream if it doesn't already exist.
// MKSTREAM creates the stream too if needed. "$" means "start reading from new
// messages only" (ignore any pre-existing backlog).
func ensureGroup(ctx context.Context, rdb *redis.Client, stream, group string) error {
	err := rdb.XGroupCreateMkStream(ctx, stream, group, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// handleEnqueue pushes a dummy job onto the jobs stream.
func (a *app) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	id, err := a.redis.XAdd(r.Context(), &redis.XAddArgs{
		Stream: queue.JobsStream,
		Values: map[string]interface{}{
			"image_id": "1",
			"note":     "dummy job from enqueue endpoint",
		},
	}).Result()
	if err != nil {
		http.Error(w, "failed to enqueue: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("enqueued job %s", id)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"job_id": id})
}

// consumeResults runs in a goroutine, continuously reading the results stream.
func (a *app) consumeResults(ctx context.Context) {
	consumerName := "backend-1"

	for {
		// Block up to 5s waiting for new entries. ">" means "undelivered
		// messages for this group" (not previously handed to any consumer).
		streams, err := a.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    queue.BackendGroup,
			Consumer: consumerName,
			Streams:  []string{queue.ResultsStream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			// redis.Nil means the 5s block elapsed with no new messages — normal.
			if err == redis.Nil {
				continue
			}
			// NOGROUP: the stream or group vanished (e.g. Redis restarted).
			// Recreate it and retry rather than erroring forever.
			if strings.Contains(err.Error(), "NOGROUP") {
				log.Println("results group missing, recreating...")
				if cerr := ensureGroup(ctx, a.redis, queue.ResultsStream, queue.BackendGroup); cerr != nil {
					log.Printf("failed to recreate results group: %v", cerr)
					time.Sleep(1 * time.Second)
				}
				continue
			}
			log.Printf("error reading results: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				log.Printf("received result %s: %v", msg.ID, msg.Values)

				// Acknowledge — tell Redis we've processed this entry.
				if err := a.redis.XAck(ctx, queue.ResultsStream, queue.BackendGroup, msg.ID).Err(); err != nil {
					log.Printf("failed to ack %s: %v", msg.ID, err)
				}
			}
		}
	}
}
