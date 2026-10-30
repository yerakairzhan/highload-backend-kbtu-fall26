package loadgen

import (
	"context"
	"fmt"

	"sis2/internal/client"
	"sis2/internal/pb"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// MessageSize is one message encoded both ways. JSON is exactly what the
// Connect JSON codec sends (protojson with the schema's json_name).
type MessageSize struct {
	Scenario    Scenario `json:"scenario"`
	Direction   string   `json:"direction"`
	Message     string   `json:"message"`
	Proto       int      `json:"proto_bytes"`
	JSON        int      `json:"json_bytes"`
	JSONToProto float64  `json:"json_to_proto"`
}

func measure(s Scenario, dir string, m proto.Message) (MessageSize, error) {
	pbBytes, err := proto.Marshal(m)
	if err != nil {
		return MessageSize{}, err
	}
	jsBytes, err := protojson.Marshal(m)
	if err != nil {
		return MessageSize{}, err
	}
	return MessageSize{
		Scenario: s, Direction: dir, Message: string(m.ProtoReflect().Descriptor().Name()),
		Proto: len(pbBytes), JSON: len(jsBytes),
		JSONToProto: float64(len(jsBytes)) / float64(max(1, len(pbBytes))),
	}, nil
}

// Sizes fetches one real request/response pair per scenario from the server
// and encodes each message both ways.
func Sizes(ctx context.Context, url, password string) ([]MessageSize, error) {
	c := client.New(url+"/api/v2", client.TargetGRPC, client.NoRetry)
	ids, err := fetchPostIds(ctx, c, 1)
	if err != nil {
		return nil, err
	}
	postId := proto.Uint64(ids[0])

	getReq := &pb.IdRequest{Id: postId}
	post, err := c.Post.GetPost(ctx, getReq)
	if err != nil {
		return nil, err
	}
	listReq := &pb.CommentListRequest{PostId: postId, Limit: proto.Int32(20)}
	comments, err := c.Comment.ListComments(ctx, listReq)
	if err != nil {
		return nil, err
	}
	createReq := &pb.CommentCreate{
		PostId: postId, Body: proto.String(commentBody),
		IdempotencyKey: proto.String("4f1c2a9e-0b7d-4c55-9d3e-2a8f3c1d6e70"), // a UUID, as a real client sends
	}
	tokens, err := loginSeedUsers(ctx, c, 1, password)
	if err != nil {
		return nil, err
	}
	created, err := c.Comment.CreateComment(client.WithToken(ctx, tokens[0]), createReq)
	if err != nil {
		return nil, err
	}

	pairs := []struct {
		s        Scenario
		req, res proto.Message
	}{
		{ScenarioGetPost, getReq, post},
		{ScenarioListComments, listReq, comments},
		{ScenarioCreateComment, createReq, created},
	}
	var out []MessageSize
	for _, p := range pairs {
		for dir, msg := range map[string]proto.Message{"request": p.req, "response": p.res} {
			size, err := measure(p.s, dir, msg)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", p.s, dir, err)
			}
			out = append(out, size)
		}
	}
	return out, nil
}
