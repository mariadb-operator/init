package environment

import (
	"context"

	"github.com/sethvargo/go-envconfig"
)

type Environment struct {
	PodName             string `env:"POD_NAME,required"`
	MariadbRootPassword string `env:"MARIADB_ROOT_PASSWORD,required"`
}

func GetEnvironment(ctx context.Context) (*Environment, error) {
	var env Environment
	if err := envconfig.Process(ctx, &env); err != nil {
		return nil, err
	}
	return &env, nil
}
