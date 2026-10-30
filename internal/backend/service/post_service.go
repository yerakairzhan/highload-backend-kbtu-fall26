package service

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"sis2/internal/cloud/storage"
	"sis2/internal/common/config"
	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/repository"
	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"

	log "github.com/sirupsen/logrus"
)

const (
	maxTitleLen = 200
	maxBodyLen  = 100_000
	maxTags     = 5
)

type PostService struct{}

var (
	postServiceInstance *PostService
	postServiceOnce     sync.Once
)

func NewPostService() *PostService {
	postServiceOnce.Do(func() {
		postServiceInstance = &PostService{}
	})
	return postServiceInstance
}

func validatePost(title, body string, tags []string) error {
	if title == "" || len(title) > maxTitleLen {
		return errs.ErrInvalidValue.AddErrf("title must be 1-%d bytes", maxTitleLen)
	}
	if len(body) > maxBodyLen {
		return errs.ErrInvalidValue.AddErrf("body must be at most %d bytes", maxBodyLen)
	}
	if len(tags) > maxTags {
		return errs.ErrInvalidValue.AddErrf("at most %d tags", maxTags)
	}
	return nil
}

// CreatePost publishes a post. With an idempotency key, a retry of the same
// request returns the post the first attempt created instead of a second one;
// the check and the insert run in one transaction, so a retry racing the
// original cannot slip between them.
func (s *PostService) CreatePost(ctx context.Context, req *pb.PostCreate) (*pb.Post, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if err = validatePost(req.GetTitle(), req.GetBody(), req.GetTags()); err != nil {
		return nil, err
	}
	key := req.GetIdempotencyKey()
	hash := requestHash(append([]string{req.GetTitle(), req.GetBody()}, req.GetTags()...)...)

	var created *models.Post
	err = database.Transactional(ctx, func(ctx context.Context) error {
		if key != "" {
			if stored := repository.IdempotencyKey().Find(ctx, user.Id, key); stored.IsOk() {
				if stored.MustGet().RequestHash != hash {
					return errs.ErrIdempotencyKeyReused.AddErrf("key %q", key)
				}
				result := repository.Post().FindById(ctx, stored.MustGet().EntityId)
				if result.IsError() {
					return result.Error()
				}
				created = result.MustGet()
				return nil
			}
		}
		created = repository.Post().Create(ctx, &models.Post{
			AuthorId:   user.Id,
			AuthorName: user.Name(),
			Title:      req.GetTitle(),
			Body:       req.GetBody(),
			Tags:       slices.Clone(req.GetTags()),
			Status:     pb.PostStatus_POST_STATUS_PUBLISHED,
			Version:    1,
		}).MustGet()
		if key != "" {
			repository.IdempotencyKey().Save(ctx, &models.IdempotencyKey{
				UserId: user.Id, Key: key, RequestHash: hash, EntityId: created.Id,
			})
		}
		return nil
	})
	if err != nil {
		log.WithContext(ctx).Warnf("Error creating post: %v", err)
		return nil, err
	}
	return created.ToProto(), nil
}

func (s *PostService) findLivePost(ctx context.Context, id uint64) (*models.Post, error) {
	result := repository.Post().FindById(ctx, id)
	if result.IsError() {
		return nil, result.Error()
	}
	post := result.MustGet()
	if post.Status == pb.PostStatus_POST_STATUS_DELETED {
		return nil, errs.ErrNotFound.AddErrf("post %d", id)
	}
	return post, nil
}

func (s *PostService) GetPost(ctx context.Context, id uint64) (*pb.Post, error) {
	post, err := s.findLivePost(ctx, id)
	if err != nil {
		return nil, err
	}
	return post.ToProto(), nil
}

// UpdatePost is an optimistic-lock write: it applies only if the caller saw
// the current version, so a concurrent edit is reported, never overwritten.
func (s *PostService) UpdatePost(ctx context.Context, req *pb.PostUpdate) (*pb.Post, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err = validatePost(req.GetTitle(), req.GetBody(), nil); err != nil {
		return nil, err
	}
	result := repository.Post().Update(ctx, req.GetId(), func(p *models.Post) error {
		switch {
		case p.Status == pb.PostStatus_POST_STATUS_DELETED:
			return errs.ErrNotFound.AddErrf("post %d", req.GetId())
		case p.AuthorId != user.Id:
			return errs.ErrPermissionDenied.AddErrf("post %d belongs to another author", p.Id)
		case p.Version != req.GetVersion():
			return errs.ErrVersionConflict.AddErrf("post %d is at version %d, not %d", p.Id, p.Version, req.GetVersion())
		}
		p.Title, p.Body, p.Version = req.GetTitle(), req.GetBody(), p.Version+1
		return nil
	})
	if result.IsError() {
		return nil, result.Error()
	}
	return result.MustGet().ToProto(), nil
}

// DeletePost is a soft delete; deleting twice is not an error, so the call is
// safe to retry.
func (s *PostService) DeletePost(ctx context.Context, id uint64) error {
	user, err := requireUser(ctx)
	if err != nil {
		return err
	}
	result := repository.Post().Update(ctx, id, func(p *models.Post) error {
		if p.AuthorId != user.Id {
			return errs.ErrPermissionDenied.AddErrf("post %d belongs to another author", p.Id)
		}
		p.Status = pb.PostStatus_POST_STATUS_DELETED
		return nil
	})
	return result.Error()
}

// ListPosts pages through published posts newest first, optionally by author
// or tag. Bodies are cut to an excerpt.
func (s *PostService) ListPosts(ctx context.Context, req *pb.PostListRequest) (*pb.PostList, error) {
	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}
	limit := pageLimit(req.GetLimit())
	authorId, tag := req.GetAuthorId(), req.GetTag()
	posts := repository.Post().Paginate(ctx, func(p *models.Post) bool {
		return p.Status == pb.PostStatus_POST_STATUS_PUBLISHED &&
			(authorId == 0 || p.AuthorId == authorId) &&
			(tag == "" || slices.Contains(p.Tags, tag))
	}, cursor, limit, true)

	out := &pb.PostList{Posts: make([]*pb.Post, len(posts))}
	for i := range posts {
		out.Posts[i] = posts[i].ToProtoExcerpt()
	}
	if len(posts) > 0 {
		out.NextCursor = nextCursor(posts[len(posts)-1].Id, len(posts), limit)
	}
	return out, nil
}

// LikePost is idempotent by construction: the (post, user) pair is the key, so
// a double click or a retry changes nothing the second time.
func (s *PostService) LikePost(ctx context.Context, postId uint64) error {
	return s.setLike(ctx, postId, true)
}

func (s *PostService) UnlikePost(ctx context.Context, postId uint64) error {
	return s.setLike(ctx, postId, false)
}

func (s *PostService) setLike(ctx context.Context, postId uint64, like bool) error {
	user, err := requireUser(ctx)
	if err != nil {
		return err
	}
	return database.Transactional(ctx, func(ctx context.Context) error {
		if _, err := s.findLivePost(ctx, postId); err != nil {
			return err
		}
		var changed bool
		if like {
			changed = repository.Like().Add(ctx, postId, user.Id).MustGet()
		} else {
			changed = repository.Like().Remove(ctx, postId, user.Id).MustGet()
		}
		if !changed {
			return nil
		}
		return repository.Post().Update(ctx, postId, func(p *models.Post) error {
			if like {
				p.LikeCount++
			} else if p.LikeCount > 0 {
				p.LikeCount--
			}
			return nil
		}).Error()
	})
}

var imageExtensions = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

// GetUploadUrl returns a presigned upload the client sends the image to
// directly (the bytes never pass through this service), and the public URL
// the post body links to.
func (s *PostService) GetUploadUrl(ctx context.Context, req *pb.UploadUrlRequest) (*pb.UploadUrl, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	ext, ok := imageExtensions[req.GetContentType()]
	if !ok {
		return nil, errs.ErrInvalidValue.AddErrf("unsupported content type %q", req.GetContentType())
	}
	store := storage.Get()
	if store == nil {
		return nil, errs.ErrUnavailable.AddErrf("object storage is not configured")
	}
	key := fmt.Sprintf("%d/%d.%s", user.Id, database.NextId(), ext)
	s3cfg := config.GetBlogPlatformConfig().S3
	upload, imageUrl, err := store.PresignUpload(ctx, key, req.GetContentType(), s3cfg.MaxUploadBytes, s3cfg.PresignTTL)
	if err != nil {
		return nil, errs.ErrUnavailable.AddErrf("presign: %v", err)
	}
	return &pb.UploadUrl{UploadUrl: &upload.URL, ImageUrl: &imageUrl, FormFields: upload.Fields}, nil
}
