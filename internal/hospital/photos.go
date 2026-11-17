package hospital

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/platform/apperr"
)

const (
	MaxPhotos     = 10
	MaxPhotoBytes = 5 << 20 // 5 MB
)

// Photo types accepted, detected from the file content.
var PhotoTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

func (s *Store) ListPhotos(ctx context.Context, hospitalID string) ([]Photo, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id FROM hospital_photos WHERE hospital_id = $1 ORDER BY created_at, id`, hospitalID)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	photos := make([]Photo, len(ids))
	for i, id := range ids {
		photos[i] = Photo{ID: id, URL: photoURL(hospitalID, id)}
	}
	return photos, nil
}

// AddPhoto stores an image for the hospital page. The first photo becomes the logo.
func (s *Store) AddPhoto(ctx context.Context, hospitalID, contentType string, r io.Reader) (Photo, error) {
	if !PhotoTypes[contentType] {
		return Photo{}, apperr.Validation("photos must be JPEG, PNG or WebP")
	}
	var count int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM hospital_photos WHERE hospital_id = $1`, hospitalID).Scan(&count); err != nil {
		return Photo{}, err
	}
	if count >= MaxPhotos {
		return Photo{}, apperr.Conflict("a hospital can have at most %d photos; delete one first", MaxPhotos)
	}

	key := uuid.NewString()
	size, err := s.files.Put(ctx, key, r)
	if err != nil {
		return Photo{}, err
	}
	if size == 0 {
		_ = s.files.Delete(ctx, key)
		return Photo{}, apperr.Validation("file is empty")
	}

	var id string
	err = s.db.QueryRow(ctx, `
		INSERT INTO hospital_photos (hospital_id, storage_key, content_type, size_bytes)
		VALUES ($1, $2, $3, $4) RETURNING id`, hospitalID, key, contentType, size).Scan(&id)
	if err != nil {
		_ = s.files.Delete(ctx, key)
		return Photo{}, err
	}
	if _, err := s.db.Exec(ctx,
		`UPDATE hospitals SET logo_photo_id = $2 WHERE id = $1 AND logo_photo_id IS NULL`, hospitalID, id); err != nil {
		return Photo{}, err
	}
	return Photo{ID: id, URL: photoURL(hospitalID, id)}, nil
}

func (s *Store) SetLogo(ctx context.Context, hospitalID, photoID string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE hospitals SET logo_photo_id = $2, updated_at = now()
		WHERE id = $1 AND EXISTS (SELECT 1 FROM hospital_photos WHERE id = $2 AND hospital_id = $1)`,
		hospitalID, photoID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("photo not found")
	}
	return nil
}

func (s *Store) DeletePhoto(ctx context.Context, hospitalID, photoID string) error {
	var key string
	err := s.db.QueryRow(ctx,
		`DELETE FROM hospital_photos WHERE id = $1 AND hospital_id = $2 RETURNING storage_key`,
		photoID, hospitalID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound("photo not found")
	}
	if err != nil {
		return err
	}
	return s.files.Delete(ctx, key)
}

// OpenPhoto returns a photo's type, size and content. Photos are public.
func (s *Store) OpenPhoto(ctx context.Context, hospitalID, photoID string) (string, int64, io.ReadCloser, error) {
	var key, contentType string
	var size int64
	err := s.db.QueryRow(ctx, `
		SELECT storage_key, content_type, size_bytes FROM hospital_photos WHERE id = $1 AND hospital_id = $2`,
		photoID, hospitalID).Scan(&key, &contentType, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, nil, apperr.NotFound("photo not found")
	}
	if err != nil {
		return "", 0, nil, err
	}
	rc, err := s.files.Open(ctx, key)
	return contentType, size, rc, err
}
