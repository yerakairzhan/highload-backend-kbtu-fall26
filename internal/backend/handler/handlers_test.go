package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sis2/internal/backend/service"
	"sis2/internal/client"
	"sis2/internal/common"
	"sis2/internal/common/config"
	"sis2/internal/pb"
	"sis2/internal/repository/database"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func startServer(t *testing.T, extra ...connect.Interceptor) string {
	database.ReloadInMemoryDB()
	InitHandlers(config.GetBlogPlatformConfig(), extra...) // test.yaml: the same JWT secret the auth service signs with
	mux := http.NewServeMux()
	MountAll(mux)
	srv := httptest.NewUnstartedServer(mux)
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetHTTP1(true)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL
}

// loseResponse runs the handler (the write commits) and then answers
// UNAVAILABLE, as if the response were lost on the way back.
func loseResponse() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if _, err := next(ctx, req); err != nil {
				return nil, err
			}
			return nil, connect.NewError(connect.CodeUnavailable, nil)
		}
	})
}

// stall holds every request for d, or until the caller's deadline passes.
func stall(d time.Duration) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			select {
			case <-time.After(d):
				return next(ctx, req)
			case <-ctx.Done():
				return nil, connect.NewError(connect.CodeDeadlineExceeded, ctx.Err())
			}
		}
	})
}

// registerUser creates a user in-process (the server shares this process)
// and returns its access token and id.
func registerUser(t *testing.T, username string) (string, uint64) {
	tokens, err := service.NewAuthService().Register(t.Context(), &pb.RegisterRequest{
		Username: proto.String(username), Password: proto.String("correct horse"),
	})
	require.NoError(t, err)
	return tokens.GetAccessToken(), tokens.GetUser().GetId()
}

func asUser(id uint64) context.Context {
	return common.WithUser(context.Background(), common.User{Id: id})
}

func TestHandlers_BothProtocolsReturnTheSamePost(t *testing.T) {
	url := startServer(t)
	require.NoError(t, service.Seed(t.Context(), config.SeedConfig{Posts: 3, CommentsPerPost: 2}))

	want := map[client.Target][2]string{
		client.TargetGRPC:   {"HTTP/2.0", "application/grpc"},
		client.TargetJSONH1: {"HTTP/1.1", "application/json"},
	}
	var first *pb.Post
	for _, target := range client.AllTargets {
		c := client.New(url, target, client.NoRetry)
		list, err := c.Post.ListPosts(t.Context(), &pb.PostListRequest{Limit: proto.Int32(1)})
		require.NoError(t, err, target)
		post, err := c.Post.GetPost(t.Context(), &pb.IdRequest{Id: list.GetPosts()[0].Id})
		require.NoError(t, err, target)

		assert.Equal(t, want[target][0], c.Probe.Proto, "%s negotiated the wrong HTTP version", target)
		assert.Equal(t, want[target][1], c.Probe.ContentType, target)
		assert.Empty(t, c.Probe.Encoding, "%s response must not be compressed", target)
		if first == nil {
			first = post
		} else {
			assert.True(t, proto.Equal(first, post), "%s returned a different post", target)
		}
	}
}

func TestHandlers_IdempotencyKeyMakesRetrySafe(t *testing.T) {
	url := startServer(t, loseResponse())
	token, userId := registerUser(t, "aidana")
	post, err := service.NewPostService().CreatePost(asUser(userId), &pb.PostCreate{Title: proto.String("T"), Body: proto.String("B")})
	require.NoError(t, err)
	retry := client.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond}
	ctx := client.WithToken(t.Context(), token)

	for _, target := range client.AllTargets {
		t.Run(string(target), func(t *testing.T) {
			keyed := client.New(url, target, retry)
			_, err := keyed.Comment.CreateComment(ctx, &pb.CommentCreate{
				PostId: post.Id, Body: proto.String("keyed " + string(target)), IdempotencyKey: proto.String("k-" + string(target)),
			})
			require.Error(t, err, "every response is lost")
			assert.Equal(t, int64(3), keyed.Retry.Attempts.Load(), "a create with a key is retried")

			unkeyed := client.New(url, target, retry)
			_, _ = unkeyed.Comment.CreateComment(ctx, &pb.CommentCreate{
				PostId: post.Id, Body: proto.String("unkeyed " + string(target)),
			})
			assert.Equal(t, int64(1), unkeyed.Retry.Attempts.Load(), "a create without a key is never retried")
		})
	}

	list, err := service.NewCommentService().ListComments(asUser(userId), &pb.CommentListRequest{PostId: post.Id, Limit: proto.Int32(100)})
	require.NoError(t, err)
	count := map[string]int{}
	for _, c := range list.GetComments() {
		count[c.GetBody()]++
	}
	for _, target := range client.AllTargets {
		assert.Equal(t, 1, count["keyed "+string(target)], "%s: 3 attempts with one key -> 1 row", target)
		assert.Equal(t, 1, count["unkeyed "+string(target)], target)
	}
}

func TestHandlers_PerAttemptTimeoutStopsWaiting(t *testing.T) {
	url := startServer(t, stall(500*time.Millisecond))
	policy := client.RetryPolicy{
		MaxAttempts: 2, PerAttemptTimeout: 50 * time.Millisecond,
		BaseBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}
	for _, target := range client.AllTargets {
		c := client.New(url, target, policy)
		start := time.Now()
		_, err := c.Post.GetPost(t.Context(), &pb.IdRequest{Id: proto.Uint64(1)})
		elapsed := time.Since(start)

		require.Error(t, err, target)
		assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(err), target)
		assert.Equal(t, int64(2), c.Retry.Attempts.Load(), "%s: a read is retried after a timeout", target)
		assert.Less(t, elapsed, 300*time.Millisecond, "%s waited for the 500 ms stall instead of its 50 ms deadline", target)
	}
}

func TestHandlers_AccessToken(t *testing.T) {
	url := startServer(t)
	token, userId := registerUser(t, "aidana")
	post, err := service.NewPostService().CreatePost(asUser(userId), &pb.PostCreate{Title: proto.String("T"), Body: proto.String("B")})
	require.NoError(t, err)

	for _, target := range client.AllTargets {
		c := client.New(url, target, client.NoRetry)

		_, err := c.Post.GetPost(t.Context(), &pb.IdRequest{Id: post.Id})
		assert.NoError(t, err, "%s: anonymous reads are allowed", target)

		_, err = c.Comment.CreateComment(t.Context(), &pb.CommentCreate{PostId: post.Id, Body: proto.String("x")})
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "%s: anonymous writes are not", target)

		_, err = c.Post.GetPost(client.WithToken(t.Context(), token+"tampered"), &pb.IdRequest{Id: post.Id})
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "%s: a bad token is rejected, not ignored", target)

		me, err := c.User.GetMe(client.WithToken(t.Context(), token), &pb.EmptyMessageRequest{})
		require.NoError(t, err, target)
		assert.Equal(t, "aidana", me.GetUsername(), target)
	}
}
