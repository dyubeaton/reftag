CREATE TABLE images (
    id         SERIAL PRIMARY KEY,
    file_path  TEXT NOT NULL UNIQUE,
    file_name  TEXT NOT NULL,
    width      INTEGER,
    height     INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);