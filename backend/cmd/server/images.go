package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Image mirrors a row in the images table
type Image struct {
	ID        int       `json:"id"`
	FilePath  string    `json:"file_path"`
	FileName  string    `json:"file_name"`
	Width     *int      `json:"width"`
	Height    *int      `json:"height"`
	CreatedAt time.Time `json:"created_at"`
}

// createImageRequest is the expected POST body
type createImageRequest struct {
	FilePath string `json:"file_path"`
	FileName string `json:"file_name"`
	Width    *int   `json:"width"`
	Height   *int   `json:"height"`
}

func (a *app) handleCreateImage(w http.ResponseWriter, r *http.Request) {
	var req createImageRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Basic validation.
	if req.FilePath == "" || req.FileName == "" {
		http.Error(w, "file_path and file_name are required", http.StatusBadRequest)
		return
	}

	const query = `
		INSERT INTO images (file_path, file_name, width, height)
		VALUES ($1, $2, $3, $4)
		RETURNING id, file_path, file_name, width, height, created_at
	`

	var img Image
	err := a.db.QueryRow(r.Context(), query,
		req.FilePath, req.FileName, req.Width, req.Height,
	).Scan(
		&img.ID, &img.FilePath, &img.FileName,
		&img.Width, &img.Height, &img.CreatedAt,
	)

	if err != nil {
		// 23505 is Postgres's unique_violation code — the file_path already exists.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "an image with that file_path already exists", http.StatusConflict)
			return
		}
		log.Printf("failed to insert image: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(img)
}
