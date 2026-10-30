package repository

func Post() *PostRepository {
	return NewPostRepository()
}

func Comment() *CommentRepository {
	return NewCommentRepository()
}

func Like() *LikeRepository {
	return NewLikeRepository()
}

func IdempotencyKey() *IdempotencyKeyRepository {
	return NewIdempotencyKeyRepository()
}

func User() *UserRepository {
	return NewUserRepository()
}

func RefreshToken() *RefreshTokenRepository {
	return NewRefreshTokenRepository()
}
