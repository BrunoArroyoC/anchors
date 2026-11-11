package functions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"

	"docker-tool/internal/models"
)

// ListImages returns the local images (top-level only, like `docker images`).
func ListImages(ctx context.Context, cli *client.Client) ([]models.ImageModel, error) {
	images, err := cli.ImageList(ctx, client.ImageListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}

	imagesList := make([]models.ImageModel, 0, len(images.Items))
	for _, v := range images.Items {
		imagesList = append(imagesList, toImage(v))
	}
	return imagesList, nil
}

func toImage(s image.Summary) models.ImageModel {
	return models.ImageModel{
		ID:            s.ID,
		ShortID:       shortID(s.ID),
		Tags:          cleanTags(s.RepoTags),
		Digests:       s.RepoDigests,
		ParentID:      s.ParentID,
		CreatedAt:     time.Unix(s.Created, 0),
		Size:          s.Size,
		NumContainers: s.Containers,
	}
}

// shortID drops the "sha256:" prefix and keeps 12 characters.
func shortID(id string) string {
	_, hex, found := strings.Cut(id, ":")
	if !found {
		hex = id
	}
	if len(hex) > 12 {
		hex = hex[:12]
	}
	return hex
}

// cleanTags drops the "<none>:<none>" placeholder so a dangling image has no tags.
func cleanTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != "<none>:<none>" {
			out = append(out, t)
		}
	}
	return out
}
