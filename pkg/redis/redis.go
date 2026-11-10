package redis

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

// ErrNil is returned when the key is not found
var ErrNil = redis.Nil

const (
	_defaultPoolSize     = 10
	_defaultConnAttempts = 10
	_defaultConnTimeout  = time.Second
)

type Redis struct {
	poolSize     int
	connAttempts int
	connTimeout  time.Duration
	db           int

	Client *redis.Client
}

func New(addr string, password string, opts ...Option) (*Redis, error) {
	r := &Redis{
		poolSize:     _defaultPoolSize,
		connAttempts: _defaultConnAttempts,
		connTimeout:  _defaultConnTimeout,
	}

	for _, opt := range opts {
		opt(r)
	}

	r.Client = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       r.db,
		PoolSize: r.poolSize,
	})

	if err := redisotel.InstrumentTracing(r.Client); err != nil {
		r.Client.Close()

		return nil, fmt.Errorf("redis - New - redisotel.InstrumentTracing: %w", err)
	}

	var err error

	for r.connAttempts > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), r.connTimeout)
		err = r.Client.Ping(ctx).Err()
		cancel()

		if err == nil {
			break
		}

		r.connAttempts--
		log.Printf("Redis is trying to connect, attempts left: %d", r.connAttempts)

		if r.connAttempts > 0 {
			time.Sleep(r.connTimeout)
		}
	}

	if err != nil {
		r.Client.Close()

		return nil, fmt.Errorf("redis - New - connAttempts == 0: %w", err)
	}

	return r, nil
}

func (r *Redis) Close() {
	if r.Client != nil {
		r.Client.Close()
	}
}
