package service

import (
	"context"
	"strconv"
	"sync"

	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/repository"
	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"

	log "github.com/sirupsen/logrus"
)

const maxCommentLen = 10_000

type CommentService struct{}

var (
	commentServiceInstance *CommentService
	commentServiceOnce     sync.Once
)

func NewCommentService() *CommentService {
	commentServiceOnce.Do(func() {
		commentServiceInstance = &CommentService{}
	})
	return commentServiceInstance
}

// CreateComment adds a comment or a reply. A reply's parent must belong to the
// same post. With an idempotency key, a retry returns the first comment.
func (s *CommentService) CreateComment(ctx context.Context, req *pb.CommentCreate) (*pb.Comment, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetBody() == "" || len(req.GetBody()) > maxCommentLen {
		return nil, errs.ErrInvalidValue.AddErrf("body must be 1-%d bytes", maxCommentLen)
	}
	key := req.GetIdempotencyKey()
	hash := requestHash(
		strconv.FormatUint(req.GetPostId(), 10),
		strconv.FormatUint(req.GetParentId(), 10),
		req.GetBody(),
	)

	var created *models.Comment
	err = database.Transactional(ctx, func(ctx context.Context) error {
		if key != "" {
			if stored := repository.IdempotencyKey().Find(ctx, user.Id, key); stored.IsOk() {
				if stored.MustGet().RequestHash != hash {
					return errs.ErrIdempotencyKeyReused.AddErrf("key %q", key)
				}
				result := repository.Comment().FindById(ctx, stored.MustGet().EntityId)
				if result.IsError() {
					return result.Error()
				}
				created = result.MustGet()
				return nil
			}
		}
		if _, err := NewPostService().findLivePost(ctx, req.GetPostId()); err != nil {
			return err
		}
		if req.ParentId != nil {
			parent := repository.Comment().FindById(ctx, req.GetParentId())
			if parent.IsError() || parent.MustGet().PostId != req.GetPostId() {
				return errs.ErrInvalidValue.AddErrf("parent %d is not a comment on post %d", req.GetParentId(), req.GetPostId())
			}
		}
		created = repository.Comment().Create(ctx, &models.Comment{
			PostId:     req.GetPostId(),
			ParentId:   req.ParentId,
			AuthorId:   user.Id,
			AuthorName: user.Name(),
			Body:       req.GetBody(),
		}).MustGet()
		if key != "" {
			repository.IdempotencyKey().Save(ctx, &models.IdempotencyKey{
				UserId: user.Id, Key: key, RequestHash: hash, EntityId: created.Id,
			})
		}
		return repository.Post().Update(ctx, req.GetPostId(), func(p *models.Post) error {
			p.CommentCount++
			return nil
		}).Error()
	})
	if err != nil {
		log.WithContext(ctx).Warnf("Error creating comment on post %d: %v", req.GetPostId(), err)
		return nil, err
	}
	return created.ToProto(), nil
}

// ListComments returns a post's comments oldest first (a thread reads top
// down); the client nests replies by parent_id.
func (s *CommentService) ListComments(ctx context.Context, req *pb.CommentListRequest) (*pb.CommentList, error) {
	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}
	if _, err = NewPostService().findLivePost(ctx, req.GetPostId()); err != nil {
		return nil, err
	}
	limit := pageLimit(req.GetLimit())
	comments := repository.Comment().ListByPost(ctx, req.GetPostId(), cursor, limit)

	out := &pb.CommentList{Comments: make([]*pb.Comment, len(comments))}
	for i := range comments {
		out.Comments[i] = comments[i].ToProto()
	}
	if len(comments) > 0 {
		out.NextCursor = nextCursor(comments[len(comments)-1].Id, len(comments), limit)
	}
	return out, nil
}

// DeleteComment leaves a tombstone so replies keep their parent. The comment's
// author or the post's author may delete; deleting twice is not an error.
func (s *CommentService) DeleteComment(ctx context.Context, id uint64) error {
	user, err := requireUser(ctx)
	if err != nil {
		return err
	}
	return database.Transactional(ctx, func(ctx context.Context) error {
		found := repository.Comment().FindById(ctx, id)
		if found.IsError() {
			return found.Error()
		}
		comment := found.MustGet()
		if comment.Deleted {
			return nil
		}
		post := repository.Post().FindById(ctx, comment.PostId)
		if comment.AuthorId != user.Id && (post.IsError() || post.MustGet().AuthorId != user.Id) {
			return errs.ErrPermissionDenied.AddErrf("comment %d", id)
		}
		if err := repository.Comment().Update(ctx, id, func(c *models.Comment) error {
			c.Deleted = true
			return nil
		}).Error(); err != nil {
			return err
		}
		return repository.Post().Update(ctx, comment.PostId, func(p *models.Post) error {
			if p.CommentCount > 0 {
				p.CommentCount--
			}
			return nil
		}).Error()
	})
}
