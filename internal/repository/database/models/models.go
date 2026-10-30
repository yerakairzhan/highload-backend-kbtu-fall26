package models

import (
	"time"

	"sis2/internal/pb"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type BasicModel struct {
	Id        uint64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (m *BasicModel) GetId() uint64 { return m.Id }

func (m *BasicModel) SetId(id uint64) { m.Id = id }

func (m *BasicModel) Touch(now time.Time) {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
}

type Post struct {
	BasicModel
	AuthorId     uint64
	AuthorName   string
	Title        string
	Body         string
	Tags         []string
	Status       pb.PostStatus
	LikeCount    uint64
	CommentCount uint64
	Version      int32 // optimistic lock
}

const postExcerptLen = 300

func (p *Post) ToProto() *pb.Post {
	return &pb.Post{
		Id:           proto.Uint64(p.Id),
		AuthorId:     proto.Uint64(p.AuthorId),
		AuthorName:   proto.String(p.AuthorName),
		Title:        proto.String(p.Title),
		Body:         proto.String(p.Body),
		Tags:         p.Tags,
		Status:       p.Status.Enum(),
		LikeCount:    proto.Uint64(p.LikeCount),
		CommentCount: proto.Uint64(p.CommentCount),
		Version:      proto.Int32(p.Version),
		CreatedAt:    timestamppb.New(p.CreatedAt),
		UpdatedAt:    timestamppb.New(p.UpdatedAt),
	}
}

// ToProtoExcerpt is the list-page shape: the body is cut to an excerpt, the
// full text comes from GetPost.
func (p *Post) ToProtoExcerpt() *pb.Post {
	out := p.ToProto()
	if len(p.Body) > postExcerptLen {
		out.Body = proto.String(p.Body[:postExcerptLen])
	}
	return out
}

type Comment struct {
	BasicModel
	PostId     uint64
	ParentId   *uint64 // nil = top level
	AuthorId   uint64
	AuthorName string
	Body       string
	Deleted    bool // tombstone: replies keep their parent
}

func (c *Comment) ToProto() *pb.Comment {
	out := &pb.Comment{
		Id:         proto.Uint64(c.Id),
		PostId:     proto.Uint64(c.PostId),
		ParentId:   c.ParentId,
		AuthorId:   proto.Uint64(c.AuthorId),
		AuthorName: proto.String(c.AuthorName),
		Body:       proto.String(c.Body),
		Deleted:    proto.Bool(c.Deleted),
		CreatedAt:  timestamppb.New(c.CreatedAt),
	}
	if c.Deleted {
		out.Body, out.AuthorName = proto.String(""), proto.String("")
	}
	return out
}

// Like is keyed by (PostId, UserId): one like per user per post.
type Like struct {
	PostId    uint64
	UserId    uint64
	CreatedAt time.Time
}

// IdempotencyKey remembers which entity a create produced, scoped to its author.
type IdempotencyKey struct {
	UserId      uint64
	Key         string
	RequestHash [32]byte // the same key with a different payload is rejected
	EntityId    uint64
	CreatedAt   time.Time
}

type User struct {
	BasicModel
	Username     string // unique
	DisplayName  string
	Bio          string
	PasswordHash []byte // bcrypt
}

// Name is how the user appears as an author.
func (u *User) Name() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

func (u *User) ToProto() *pb.User {
	out := &pb.User{
		Id:        proto.Uint64(u.Id),
		Username:  proto.String(u.Username),
		CreatedAt: timestamppb.New(u.CreatedAt),
	}
	if u.DisplayName != "" {
		out.DisplayName = proto.String(u.DisplayName)
	}
	if u.Bio != "" {
		out.Bio = proto.String(u.Bio)
	}
	return out
}

// RefreshToken is stored by the SHA-256 of the token, never the token itself.
// Tokens of one login share a Family: presenting a token that was already
// used revokes the whole family, since only a thief would hold an old one.
type RefreshToken struct {
	Hash      [32]byte
	UserId    uint64
	Family    [16]byte
	ExpiresAt time.Time
	Used      bool
	Revoked   bool
}
