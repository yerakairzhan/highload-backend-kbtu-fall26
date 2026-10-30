package handler

import (
	"context"

	"sis2/internal/backend/service"
	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/pb/pbconnect"
)

// PostHandler implements pbconnect.PostServiceHandler.
type PostHandler struct {
	pbconnect.UnimplementedPostServiceHandler
	postService *service.PostService
}

func NewPostHandler() *PostHandler {
	return &PostHandler{postService: service.NewPostService()}
}

// CreatePost publishes a post
func (h *PostHandler) CreatePost(ctx context.Context, req *pb.PostCreate) (*pb.Post, error) {
	post, err := h.postService.CreatePost(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return post, nil
}

// GetPost returns one post
func (h *PostHandler) GetPost(ctx context.Context, req *pb.IdRequest) (*pb.Post, error) {
	post, err := h.postService.GetPost(ctx, req.GetId())
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return post, nil
}

// UpdatePost edits a post
func (h *PostHandler) UpdatePost(ctx context.Context, req *pb.PostUpdate) (*pb.Post, error) {
	post, err := h.postService.UpdatePost(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return post, nil
}

// DeletePost soft-deletes a post
func (h *PostHandler) DeletePost(ctx context.Context, req *pb.IdRequest) (*pb.EmptyMessageResponse, error) {
	if err := h.postService.DeletePost(ctx, req.GetId()); err != nil {
		return nil, errs.ConnectErr(err)
	}
	return &pb.EmptyMessageResponse{}, nil
}

// ListPosts pages through posts
func (h *PostHandler) ListPosts(ctx context.Context, req *pb.PostListRequest) (*pb.PostList, error) {
	posts, err := h.postService.ListPosts(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return posts, nil
}

// LikePost likes a post
func (h *PostHandler) LikePost(ctx context.Context, req *pb.PostLikeRequest) (*pb.EmptyMessageResponse, error) {
	if err := h.postService.LikePost(ctx, req.GetPostId()); err != nil {
		return nil, errs.ConnectErr(err)
	}
	return &pb.EmptyMessageResponse{}, nil
}

// UnlikePost removes a like
func (h *PostHandler) UnlikePost(ctx context.Context, req *pb.PostLikeRequest) (*pb.EmptyMessageResponse, error) {
	if err := h.postService.UnlikePost(ctx, req.GetPostId()); err != nil {
		return nil, errs.ConnectErr(err)
	}
	return &pb.EmptyMessageResponse{}, nil
}

// GetUploadUrl returns a presigned image upload URL
func (h *PostHandler) GetUploadUrl(ctx context.Context, req *pb.UploadUrlRequest) (*pb.UploadUrl, error) {
	url, err := h.postService.GetUploadUrl(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return url, nil
}
