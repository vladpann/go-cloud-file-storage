package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"

	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

const (
	_defaultAddr              = ":80"
	_defaultReadHeaderTimeout = 5 * time.Second
	_defaultReadTimeout       = 5 * time.Second
	_defaultWriteTimeout      = 5 * time.Second
	_defaultShutdownTimeout   = 3 * time.Second
)

type Server struct {
	ctx context.Context
	eg  *errgroup.Group

	Router *chi.Mux
	server *http.Server
	notify chan error

	address           string
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	shutdownTimeout   time.Duration

	logger logger.Interface
}

func New(l logger.Interface, opts ...Option) *Server {
	group, ctx := errgroup.WithContext(context.Background())
	group.SetLimit(1) // Run only one goroutine

	s := &Server{
		ctx:               ctx,
		eg:                group,
		Router:            chi.NewRouter(),
		notify:            make(chan error, 1),
		address:           _defaultAddr,
		readHeaderTimeout: _defaultReadHeaderTimeout,
		readTimeout:       _defaultReadTimeout,
		writeTimeout:      _defaultWriteTimeout,
		shutdownTimeout:   _defaultShutdownTimeout,
		logger:            l,
	}

	// Custom options
	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Server) Start() {
	s.server = &http.Server{
		Addr:              s.address,
		Handler:           s.Router,
		ReadHeaderTimeout: s.readHeaderTimeout,
		ReadTimeout:       s.readTimeout,
		WriteTimeout:      s.writeTimeout,
	}

	s.eg.Go(func() error {
		err := s.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.notify <- err

			close(s.notify)

			return err
		}

		return nil
	})

	s.logger.Info("httpserver - Server - Started")
}

func (s *Server) Notify() <-chan error {
	return s.notify
}

func (s *Server) Shutdown() error {
	var shutdownErrors []error

	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	err := s.server.Shutdown(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Error(err, "httpserver - Server - Shutdown - s.server.Shutdown")

		shutdownErrors = append(shutdownErrors, err)
	}

	// Wait for all goroutines to finish and get any error
	err = s.eg.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Error(err, "httpserver - Server - Shutdown - s.eg.Wait")

		shutdownErrors = append(shutdownErrors, err)
	}

	s.logger.Info("httpserver - Server - Shutdown")

	return errors.Join(shutdownErrors...)
}
