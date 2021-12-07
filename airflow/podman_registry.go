package airflow

import (
	"context"
	"os"

	"github.com/containers/common/pkg/auth"
)

type PodmanRegistry struct {
	conn       context.Context
	podmanBind PodmanBind
	registry   string
}

func PodmanRegistryInit(conn context.Context, registry string) (*PodmanRegistry, error) {
	binder := PodmanBinder{}
	if conn == nil {
		var err error
		conn, err = getConn(context.TODO(), &binder)
		if err != nil {
			return nil, err
		}
	}

	// We use latest and keep this tag around after deployments to keep subsequent deploys quick
	return &PodmanRegistry{conn: conn, registry: registry, podmanBind: &binder}, nil
}

func (p *PodmanRegistry) Login(username, token string) error {
	options := &auth.LoginOptions{
		Username:                  username,
		Password:                  token,
		Stdin:                     os.Stdin,
		Stdout:                    os.Stdout,
		AcceptUnspecifiedRegistry: true,
		AcceptRepositories:        true,
	}
	return p.podmanBind.Login(p.conn, nil, options, []string{p.registry})
}
