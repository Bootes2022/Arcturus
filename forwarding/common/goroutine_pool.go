package common

import (
	"github.com/panjf2000/ants/v2"
	"log"
)

type PoolConfig struct {
	MaxWorkers int
}

func NewPool(config PoolConfig) (*ants.Pool, error) {

	pool, err := ants.NewPool(config.MaxWorkers)
	if err != nil {
		log.Fatalf("Failed to create ants pool_manager: %v", err)
		return nil, err
	}

	return pool, nil
}
