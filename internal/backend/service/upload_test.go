package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"sis2/internal/cloud/storage"
	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/repository/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type fakeStorage struct {
	key, contentType string
	maxBytes         int64
}

func (f *fakeStorage) PresignUpload(_ context.Context, key, contentType string, maxBytes int64, _ time.Duration) (storage.Upload, string, error) {
	f.key, f.contentType, f.maxBytes = key, contentType, maxBytes
	return storage.Upload{URL: "http://store/images", Fields: map[string]string{"key": key}}, "http://store/images/" + key, nil
}

func TestPostService_GetUploadUrl(t *testing.T) {
	database.ReloadInMemoryDB()
	ctx := asUser(t, 9)
	req := &pb.UploadUrlRequest{ContentType: proto.String("image/png")}

	storage.Set(nil)
	_, err := NewPostService().GetUploadUrl(ctx, req)
	assert.ErrorIs(t, err, errs.ErrUnavailable)

	fake := &fakeStorage{}
	storage.Set(fake)
	t.Cleanup(func() { storage.Set(nil) })
	url, err := NewPostService().GetUploadUrl(ctx, req)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(fake.key, "9/") && strings.HasSuffix(fake.key, ".png"), fake.key)
	assert.Equal(t, "image/png", fake.contentType, "the content type goes into the signed policy")
	assert.Positive(t, fake.maxBytes, "the size limit goes into the signed policy")
	assert.Equal(t, fake.key, url.GetFormFields()["key"])
	assert.Equal(t, "http://store/images/"+fake.key, url.GetImageUrl())

	_, err = NewPostService().GetUploadUrl(ctx, &pb.UploadUrlRequest{ContentType: proto.String("text/html")})
	assert.ErrorIs(t, err, errs.ErrInvalidValue)
}
