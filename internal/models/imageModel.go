package models

import "time"

// ImageModel is one local Docker image.
// https://pkg.go.dev/github.com/moby/moby/api/types/image#Summary
type ImageModel struct {
	ID            string    // full ID, e.g. "sha256:3f57d9401f8d…"
	ShortID       string    // first 12 hex characters of the ID, as `docker images` shows it
	Tags          []string  // RepoTags, e.g. "nginx:1.27"; empty for a dangling image
	Digests       []string  // RepoDigests, e.g. "nginx@sha256:…"
	ParentID      string    // only set for locally built images; empty for pulled ones
	CreatedAt     time.Time // when the image was created
	Size          int64     // total size in bytes, all layers included
	NumContainers int64
}
