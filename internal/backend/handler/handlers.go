package handler

import (
	"net/http"

	"sis2/internal/backend/interceptor"
	"sis2/internal/common/config"
	"sis2/internal/pb/pbconnect"

	"connectrpc.com/connect"
	log "github.com/sirupsen/logrus"
)

type registeredHandler struct {
	path    string
	handler http.Handler
}

var registeredHandlers []registeredHandler

// InitHandlers builds every service handler. One Connect handler answers both
// wire protocols on the same path: gRPC, and Connect (JSON over HTTP/1.1).
// extra interceptors run after the standard ones; tests use them to inject
// failures.
func InitHandlers(cfg *config.BlogPlatformConfig, extra ...connect.Interceptor) {
	stack := append([]connect.Interceptor{
		interceptor.TimeoutInterceptor(cfg.RpcTimeout),
		interceptor.AuthInterceptor(&cfg.JWT),
	}, extra...)
	opts := connect.WithInterceptors(stack...)

	registeredHandlers = registeredHandlers[:0]
	register := func(path string, h http.Handler) {
		registeredHandlers = append(registeredHandlers, registeredHandler{path, h})
	}
	register(pbconnect.NewAuthServiceHandler(NewAuthHandler(), opts))
	register(pbconnect.NewUserServiceHandler(NewUserHandler(), opts))
	register(pbconnect.NewPostServiceHandler(NewPostHandler(), opts))
	register(pbconnect.NewCommentServiceHandler(NewCommentHandler(), opts))
}

func MountAll(mux *http.ServeMux) {
	for _, r := range registeredHandlers {
		log.Infof("Registering handler at path: %s", r.path)
		mux.Handle(r.path, r.handler)
	}
}
