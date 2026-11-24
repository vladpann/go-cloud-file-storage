package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-playground/validator/v10"

	"github.com/vladpann/go-cloud-file-storage/config"
	core_http "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	core_http_v1 "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1"
	resources_repository_minio "github.com/vladpann/go-cloud-file-storage/internal/features/resources/repository/minio"
	resources_service "github.com/vladpann/go-cloud-file-storage/internal/features/resources/service"
	resources_transport_http "github.com/vladpann/go-cloud-file-storage/internal/features/resources/transport/http"
	sessions_repository_redis "github.com/vladpann/go-cloud-file-storage/internal/features/sessions/repository/redis"
	sessions_service "github.com/vladpann/go-cloud-file-storage/internal/features/sessions/service"
	users_repository_postgres "github.com/vladpann/go-cloud-file-storage/internal/features/users/repository/postgres"
	users_service "github.com/vladpann/go-cloud-file-storage/internal/features/users/service"
	users_transport_http "github.com/vladpann/go-cloud-file-storage/internal/features/users/transport/http"
	"github.com/vladpann/go-cloud-file-storage/pkg/httpserver"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
	"github.com/vladpann/go-cloud-file-storage/pkg/minio"
	"github.com/vladpann/go-cloud-file-storage/pkg/postgres"
	"github.com/vladpann/go-cloud-file-storage/pkg/redis"
	"github.com/vladpann/go-cloud-file-storage/pkg/tracing"
)

type features struct {
	users     *users_service.UsersService
	sessions  *sessions_service.SessionsService
	resources *resources_service.ResourcesService
}

type servers struct {
	http *httpserver.Server
}

func initFeatures(cfg *config.Config, pg *postgres.Postgres, rdb *redis.Redis, mc *minio.Client) features {
	usersRepository := users_repository_postgres.New(pg)
	passwordHasher := users_service.NewPasswordHasher()
	usersService := users_service.New(usersRepository, passwordHasher)

	sessionsRepository := sessions_repository_redis.New(rdb)
	sessionsService := sessions_service.New(cfg, sessionsRepository)

	resourcesRepository := resources_repository_minio.New(mc)
	resourcesService := resources_service.New(cfg, resourcesRepository)

	return features{
		users:     usersService,
		sessions:  sessionsService,
		resources: resourcesService,
	}
}

func initV1HTTPHandlers(cfg *config.Config, feat features, l logger.Interface) *core_http_v1.V1 {
	authMiddleware := core_http_middleware.Auth(l, feat.sessions)

	// Create shared validator
	v := validator.New(validator.WithRequiredStructEnabled())

	// Initialize transports
	usersTransport := users_transport_http.NewUsersHTTPHandler(
		feat.users,
		feat.sessions,
		feat.resources,
		authMiddleware,
		cfg,
		l,
		v,
	)

	resourcesTransport := resources_transport_http.NewResourcesHTTPHandler(
		feat.resources,
		cfg,
		l,
		v,
	)

	return core_http_v1.NewV1(usersTransport, resourcesTransport, l)
}

func initServers(cfg *config.Config, v1Transports *core_http_v1.V1, l logger.Interface) servers {
	// HTTP Server
	httpServer := httpserver.New(l, httpserver.Port(cfg.HTTP.Port))
	core_http.NewRouter(httpServer.Router, cfg, v1Transports, l)

	return servers{
		http: httpServer,
	}
}

func (s *servers) startServers() {
	s.http.Start()
}

func (s *servers) waitForShutdown(l logger.Interface) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	var err error

	select {
	case sig := <-interrupt:
		l.Info("app - Run - signal: %s", sig.String())
	case err = <-s.http.Notify():
		l.Error(fmt.Errorf("app - Run - httpServer.Notify: %w", err))
	}

	s.shutdownServers(l)
}

func (s *servers) shutdownServers(l logger.Interface) {
	if err := s.http.Shutdown(); err != nil {
		l.Error(fmt.Errorf("app - Run - httpServer.Shutdown: %w", err))
	}
}

func Run(cfg *config.Config) {
	l, err := logger.New(cfg.Log.Level, cfg.Log.Folder)
	if err != nil {
		log.Fatalf("app - Run - logger.New: %v", err)
	}
	defer l.Close()

	ctx := context.Background()

	// Tracing
	shutdownTracing, err := tracing.New(ctx, tracing.Config{
		Enabled:     cfg.Tracing.Enabled,
		ServiceName: cfg.App.Name,
		Version:     cfg.App.Version,
		Endpoint:    cfg.Tracing.OTLPEndpoint,
		Insecure:    cfg.Tracing.OTLPInsecure,
		SampleRate:  cfg.Tracing.SampleRate,
	})
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - tracing.New: %w", err))
	}
	defer func() {
		if err := shutdownTracing(ctx); err != nil {
			l.Error(fmt.Errorf("app - Run - shutdownTracing: %w", err))
		}
	}()

	// Repository
	// PG
	pg, err := postgres.New(cfg.PG.URL, postgres.MaxPoolSize(cfg.PG.PoolMax))
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - postgres.New: %w", err))
	}
	defer pg.Close()

	// Redis
	rdb, err := redis.New(cfg.Redis.Addr, cfg.Redis.Password)
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - redis.New: %w", err))
	}
	defer rdb.Close()

	// MinIO
	mc, err := minio.New(ctx, minio.Config{
		Endpoint:        cfg.MinIO.Endpoint,
		AccessKeyID:     cfg.MinIO.AccessKeyID,
		SecretAccessKey: cfg.MinIO.SecretAccessKey,
		UseSSL:          cfg.MinIO.UseSSL,
		BucketName:      cfg.MinIO.BucketName,
	})
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - minio.New: %w", err))
	}

	f := initFeatures(cfg, pg, rdb, mc)
	v1HTTPHandlers := initV1HTTPHandlers(cfg, f, l)
	s := initServers(cfg, v1HTTPHandlers, l)
	s.startServers()
	s.waitForShutdown(l)
}
