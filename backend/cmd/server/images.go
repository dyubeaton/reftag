package main

import "time"

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
