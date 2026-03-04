package rediskit

import (
	"context"
	"crypto/tls"

	"github.com/redis/go-redis/v9"
)

func NewClient(redisURL string, tlsInsecure bool) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	if opt.TLSConfig != nil {
		clone := opt.TLSConfig.Clone()
		clone.InsecureSkipVerify = tlsInsecure
		opt.TLSConfig = clone
	} else if tlsInsecure {
		opt.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := redis.NewClient(opt)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}
