package repository

import (
	"sync"

	"sis2/internal/repository/database/models"
)

type PostRepository struct {
	BaseRepository[models.Post, *models.Post]
}

var (
	postRepositoryInstance *PostRepository
	postRepositoryOnce     sync.Once
)

func NewPostRepository() *PostRepository {
	postRepositoryOnce.Do(func() {
		postRepositoryInstance = &PostRepository{newBaseRepository[models.Post, *models.Post]()}
	})
	return postRepositoryInstance
}
