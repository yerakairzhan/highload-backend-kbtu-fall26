package handler

import (
	"context"

	"sis2/internal/backend/service"
	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/pb/pbconnect"
)

// CommentHandler implements pbconnect.CommentServiceHandler.
type CommentHandler struct {
	pbconnect.UnimplementedCommentServiceHandler
	commentService *service.CommentService
}

func NewCommentHandler() *CommentHandler {
	return &CommentHandler{commentService: service.NewCommentService()}
}

// CreateComment comments on a post
func (h *CommentHandler) CreateComment(ctx context.Context, req *pb.CommentCreate) (*pb.Comment, error) {
	comment, err := h.commentService.CreateComment(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return comment, nil
}

// ListComments pages through a post's comments
func (h *CommentHandler) ListComments(ctx context.Context, req *pb.CommentListRequest) (*pb.CommentList, error) {
	comments, err := h.commentService.ListComments(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return comments, nil
}

// DeleteComment deletes a comment
func (h *CommentHandler) DeleteComment(ctx context.Context, req *pb.IdRequest) (*pb.EmptyMessageResponse, error) {
	if err := h.commentService.DeleteComment(ctx, req.GetId()); err != nil {
		return nil, errs.ConnectErr(err)
	}
	return &pb.EmptyMessageResponse{}, nil
}
