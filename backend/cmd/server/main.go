package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
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
