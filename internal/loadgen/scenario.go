package loadgen

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"sis2/internal/client"
	"sis2/internal/pb"

	"google.golang.org/protobuf/proto"
)

type Scenario string

const (
	ScenarioGetPost       Scenario = "get_post"       // ~10 KB response, mostly text
	ScenarioListComments  Scenario = "list_comments"  // 20 small messages: ids, timestamps, ~300 B text
	ScenarioCreateComment Scenario = "create_comment" // write with an idempotency key
)

var AllScenarios = []Scenario{ScenarioGetPost, ScenarioListComments, ScenarioCreateComment}

// commentBody is the same 300 bytes for every protocol and every call.
var commentBody = strings.Repeat("retry with the same idempotency key. ", 10)[:300]

// fetchPostIds pages through the seeded posts; it runs before measuring.
func fetchPostIds(ctx context.Context, c *client.Clients, n int) ([]uint64, error) {
	var ids []uint64
	req := &pb.PostListRequest{Limit: proto.Int32(100)}
	for len(ids) < n {
		page, err := c.Post.ListPosts(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, p := range page.GetPosts() {
			ids = append(ids, p.GetId())
		}
		if page.NextCursor == nil {
			break
		}
		req.Cursor = page.NextCursor
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("server has no posts: is seeding enabled?")
	}
	return ids, nil
}

// loginSeedUsers logs in the first n seeded users and returns their access
// tokens. Every benchmark request carries one of them, as a signed-in app's
// requests do.
func loginSeedUsers(ctx context.Context, c *client.Clients, n int, password string) ([]string, error) {
	tokens := make([]string, n)
	for i := range tokens {
		res, err := c.Auth.Login(ctx, &pb.LoginRequest{
			Username: proto.String(fmt.Sprintf("user_%04d", i+1)), Password: proto.String(password),
		})
		if err != nil {
			return nil, fmt.Errorf("login user_%04d: %w", i+1, err)
		}
		tokens[i] = res.GetAccessToken()
	}
	return tokens, nil
}

// call returns one request of the scenario. nonce makes idempotency keys
// unique across runs against the same server.
func (s Scenario) call(c *client.Clients, postIds []uint64, tokens []string, nonce string) func(ctx context.Context, i int) error {
	pick := func() *uint64 { return proto.Uint64(postIds[rand.IntN(len(postIds))]) }
	signedIn := func(ctx context.Context) context.Context {
		return client.WithToken(ctx, tokens[rand.IntN(len(tokens))])
	}
	switch s {
	case ScenarioGetPost:
		return func(ctx context.Context, _ int) error {
			_, err := c.Post.GetPost(signedIn(ctx), &pb.IdRequest{Id: pick()})
			return err
		}
	case ScenarioListComments:
		return func(ctx context.Context, _ int) error {
			_, err := c.Comment.ListComments(signedIn(ctx), &pb.CommentListRequest{PostId: pick(), Limit: proto.Int32(20)})
			return err
		}
	case ScenarioCreateComment:
		return func(ctx context.Context, i int) error {
			_, err := c.Comment.CreateComment(signedIn(ctx), &pb.CommentCreate{
				PostId: pick(), Body: proto.String(commentBody),
				IdempotencyKey: proto.String(fmt.Sprintf("%s-%08d", nonce, i)),
			})
			return err
		}
	}
	panic("unknown scenario " + string(s))
}
