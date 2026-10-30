package platform

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"sis2/internal/backend/handler"
	"sis2/internal/backend/service"
	"sis2/internal/cloud/storage"
	"sis2/internal/common/config"
	logger "sis2/internal/common/log"

	log "github.com/sirupsen/logrus"
)

// Protocols serves HTTP/1.1 and cleartext HTTP/2 (h2c) on the same port, so
// the gRPC and the JSON-over-HTTP/1.1 clients reach the very same handlers.
func Protocols() *http.Protocols {
	var p http.Protocols
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return &p
}

type BlogPlatform struct {
	cfg     *config.BlogPlatformConfig
	mux     *http.ServeMux
	server  *http.Server
	running atomic.Bool
}

func NewBlogPlatform() Platform {
	cfg := config.GetBlogPlatformConfig()
	logger.InitLogger(cfg.Log.Level)

	if cfg.Seed.Enabled {
		if err := service.Seed(context.Background(), cfg.Seed); err != nil {
			log.Errorf("Failed to seed the store: %v", err)
			return nil
		}
		log.Infof("Seeded %d users, %d posts x %d comments", cfg.Seed.Users, cfg.Seed.Posts, cfg.Seed.CommentsPerPost)
	}

	if cfg.S3.Enabled {
		s3, err := connectStorage(&cfg.S3)
		if err != nil {
			log.Errorf("Failed to connect object storage: %v", err)
			return nil
		}
		storage.Set(s3)
		log.Infof("Object storage ready: bucket %q at %s", cfg.S3.Bucket, cfg.S3.Endpoint)
	}

	handler.InitHandlers(cfg)
	apiMux := http.NewServeMux()
	handler.MountAll(apiMux)

	mux := http.NewServeMux()
	mux.Handle(apiPrefix, http.StripPrefix("/api/v2", apiMux))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return &BlogPlatform{cfg: cfg, mux: mux}
}

// connectStorage retries for a while: the object store may still be starting.
func connectStorage(cfg *config.S3Config) (*storage.S3, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s3, err := storage.NewS3(ctx, cfg)
		cancel()
		if err == nil || time.Now().After(deadline) {
			return s3, err
		}
		log.Warnf("Object storage not ready, retrying: %v", err)
		time.Sleep(time.Second)
	}
}

func (p *BlogPlatform) Start() error {
	addr := ":" + p.cfg.ServePort
	log.Infof("Starting server on %s", addr)

	p.server = &http.Server{
		Addr:         addr,
		Handler:      p.mux,
		Protocols:    Protocols(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errChan := make(chan error, 1)
	go func() {
		p.running.Store(true)
		if err := p.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
		p.running.Store(false)
	}()

	select {
	case err := <-errChan:
		return err
	case <-time.After(100 * time.Millisecond):
		log.Infof("Server started successfully on %s", addr)
		return nil
	}
}

func (p *BlogPlatform) Stop() error {
	if p.server == nil {
		return errors.New("server is not running or not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.server.Shutdown(ctx)
}

func (p *BlogPlatform) IsRunning() bool {
	return p.running.Load()
}
