package service

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"sis2/internal/common/config"
	"sis2/internal/pb"
	"sis2/internal/repository"
	"sis2/internal/repository/database/models"

	"golang.org/x/crypto/bcrypt"
)

const (
	seedPostBodyBytes = 10_000 // SIS #1: an average post is ~10 KB of markdown
	seedCommentBytes  = 300
)

var seedWords = strings.Fields(`the a of to and in is that for on with as by it this be are from at or an
was which can not we our you your if but all will one has more about when there their so they
database index query cache latency throughput replica primary shard partition transaction commit
lock isolation snapshot vacuum backup failover leader follower quorum consensus raft clock event
queue topic offset consumer producer broker stream batch window retry timeout deadline backoff
jitter circuit breaker bulkhead limiter token bucket gateway proxy balancer service pod container
cluster node region zone network packet socket handshake protocol frame header payload encoding
schema field message request response client server thread goroutine channel mutex atomic memory`)

var seedTags = []string{"go", "grpc", "postgres", "redis", "kafka", "kubernetes", "caching",
	"latency", "scaling", "sharding", "replication", "http2", "protobuf", "observability",
	"testing", "security", "networking", "databases", "design", "career"}

func seedText(r *rand.Rand, n int) string {
	var b strings.Builder
	for b.Len() < n {
		if b.Len() > 0 {
			if r.IntN(12) == 0 {
				b.WriteString(".\n\n")
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(seedWords[r.IntN(len(seedWords))])
	}
	return b.String()[:n]
}

// SeedUsername is the name of the i-th seeded user (1-based); they all share
// cfg.UserPassword, so the load generator can log in as any of them.
func SeedUsername(i int) string {
	return fmt.Sprintf("user_%04d", i)
}

// Seed fills the store from a fixed RNG seed, so every run and every protocol
// reads exactly the same bytes. It writes through the repositories directly:
// seeding is a bulk load, not user traffic.
func Seed(ctx context.Context, cfg config.SeedConfig) error {
	r := rand.New(rand.NewPCG(42, 2026))
	// One bcrypt hash for everyone: hashing per user would make startup slow.
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.UserPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	users := make([]*models.User, max(1, cfg.Users))
	for i := range users {
		result := repository.User().Create(ctx, &models.User{Username: SeedUsername(i + 1), PasswordHash: hash})
		if result.IsError() {
			return result.Error()
		}
		users[i] = result.MustGet()
	}
	pickUser := func() *models.User { return users[r.IntN(len(users))] }

	for range cfg.Posts {
		tags := make([]string, 0, 3)
		for len(tags) < 3 {
			if t := seedTags[r.IntN(len(seedTags))]; !containsTag(tags, t) {
				tags = append(tags, t)
			}
		}
		title := strings.TrimSpace(seedText(r, 40+r.IntN(40)))
		author := pickUser()
		post := repository.Post().Create(ctx, &models.Post{
			AuthorId:   author.Id,
			AuthorName: author.Name(),
			Title:      title,
			Body:       "# " + title + "\n\n" + seedText(r, seedPostBodyBytes),
			Tags:       tags,
			Status:     pb.PostStatus_POST_STATUS_PUBLISHED,
			Version:    1,
		}).MustGet()

		var topLevel []uint64
		for range cfg.CommentsPerPost {
			var parent *uint64
			if len(topLevel) > 0 && r.IntN(5) == 0 { // ~20 % are replies
				p := topLevel[r.IntN(len(topLevel))]
				parent = &p
			}
			commenter := pickUser()
			c := repository.Comment().Create(ctx, &models.Comment{
				PostId:     post.Id,
				ParentId:   parent,
				AuthorId:   commenter.Id,
				AuthorName: commenter.Name(),
				Body:       seedText(r, seedCommentBytes),
			}).MustGet()
			if parent == nil {
				topLevel = append(topLevel, c.Id)
			}
		}
		repository.Post().Update(ctx, post.Id, func(p *models.Post) error {
			p.CommentCount = uint64(cfg.CommentsPerPost)
			return nil
		})
	}
	return nil
}

func containsTag(tags []string, t string) bool {
	for _, x := range tags {
		if x == t {
			return true
		}
	}
	return false
}
