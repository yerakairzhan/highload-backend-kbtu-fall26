package client

import (
	"context"

	"sis2/internal/pb/pbconnect"

	"connectrpc.com/connect"
)

// Target is one wire protocol under test.
type Target string

const (
	TargetGRPC   Target = "grpc"    // gRPC framing + protobuf, HTTP/2
	TargetJSONH1 Target = "json_h1" // Connect protocol + JSON, HTTP/1.1: the REST/JSON baseline
)

var AllTargets = []Target{TargetGRPC, TargetJSONH1}

func (t Target) HTTPVersion() HTTPVersion {
	if t == TargetGRPC {
		return H2C
	}
	return HTTP1
}

func (t Target) protocolOptions() []connect.ClientOption {
	if t == TargetGRPC {
		return []connect.ClientOption{connect.WithGRPC()}
	}
	return []connect.ClientOption{connect.WithProtoJSON()}
}

// Clients are the service stubs plus what was measured on their wire.
type Clients struct {
	Auth    pbconnect.AuthServiceClient
	User    pbconnect.UserServiceClient
	Post    pbconnect.PostServiceClient
	Comment pbconnect.CommentServiceClient
	Wire    *WireCounter
	Probe   *ProtoProbe
	Retry   *RetryStats
}

type tokenKey struct{}

// WithToken makes the calls made with ctx carry this access token.
func WithToken(ctx context.Context, accessToken string) context.Context {
	return context.WithValue(ctx, tokenKey{}, accessToken)
}

// bearerInterceptor sends the context's access token as Authorization: Bearer.
func bearerInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if token, ok := ctx.Value(tokenKey{}).(string); ok && token != "" {
				req.Header().Set("Authorization", "Bearer "+token)
			}
			return next(ctx, req)
		}
	})
}

func New(baseURL string, target Target, policy RetryPolicy) *Clients {
	c := &Clients{Wire: &WireCounter{}, Probe: &ProtoProbe{}, Retry: &RetryStats{}}
	httpClient := NewHTTPClient(target.HTTPVersion(), c.Wire, c.Probe)
	opts := append(target.protocolOptions(),
		// No client compression either way: the comparison is raw encodings.
		connect.WithAcceptCompression("gzip", nil, nil),
		connect.WithInterceptors(RetryInterceptor(policy, c.Retry), bearerInterceptor()),
	)
	c.Auth = pbconnect.NewAuthServiceClient(httpClient, baseURL, opts...)
	c.User = pbconnect.NewUserServiceClient(httpClient, baseURL, opts...)
	c.Post = pbconnect.NewPostServiceClient(httpClient, baseURL, opts...)
	c.Comment = pbconnect.NewCommentServiceClient(httpClient, baseURL, opts...)
	return c
}
