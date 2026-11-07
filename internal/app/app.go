package app

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vladpann/go-cloud-file-storage/config"
	core_http "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http"
	"github.com/vladpann/go-cloud-file-storage/pkg/httpserver"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
	"github.com/vladpann/go-cloud-file-storage/pkg/postgres"
)

type features struct{}

type servers struct {
	http *httpserver.Server
}

func initFeatures(pg *postgres.Postgres) features {
	return features{}
}

func initServers(cfg *config.Config, feat features, l logger.Interface) servers {
	// HTTP Server
	httpServer := httpserver.New(l, httpserver.Port(cfg.HTTP.Port))
	core_http.NewRouter(httpServer.Router, cfg, l)

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

	//ctx := context.Background()

	// Repository
	pg, err := postgres.New(cfg.PG.URL, postgres.MaxPoolSize(cfg.PG.PoolMax))
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - postgres.New: %w", err))
	}
	defer pg.Close()

	f := initFeatures(pg)
	s := initServers(cfg, f, l)
	s.startServers()
	s.waitForShutdown(l)
}
