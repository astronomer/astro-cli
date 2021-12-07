package podman

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/containers/image/v5/types"
	"github.com/containers/podman/v3/pkg/bindings"
	dockerAPITypes "github.com/docker/docker/api/types"
)

const (
	XRegistryAuthHeader = "X-Registry-Auth"
)

// PushOptions are optional options for importing images
type PushOptions struct {
	// All indicates whether to push all images related to the image list
	All *bool
	// Authfile is the path to the authentication file. Ignored for remote
	// calls.
	Authfile *string
	// Compress tarball image layers when pushing to a directory using the 'dir' transport.
	Compress *bool
	// Manifest type of the pushed image
	Format *string
	// Password for authenticating against the registry.
	Password *string
	// SkipTLSVerify to skip HTTPS and certificate verification.
	SkipTLSVerify *bool
	// Username for authenticating against the registry.
	Username *string
	// IdentityToken for authenticating against the registry.
	IdentityToken *string
}

// Push is the binding for libpod's v2 endpoints for push images.  Note that
// `source` must be a referring to an image in the remote's container storage.
// The destination must be a reference to a registry (i.e., of docker transport
// or be normalized to one).  Other transports are rejected as they do not make
// sense in a remote context. The binding is inline with podman's image binding
// package with an addition to allow the caller to pass the identity token as
// part of PushOptions.
func Push(ctx context.Context, source, destination string, options *PushOptions) error {
	if options == nil {
		options = new(PushOptions)
	}
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return err
	}
	// TODO: have a global system context we can pass around (1st argument)
	header, err := authHeader(options.GetIdentityToken())
	if err != nil {
		return err
	}

	params, err := options.ToParams()
	if err != nil {
		return err
	}
	// SkipTLSVerify is special.  We need to delete the param added by
	// toparams and change the key and flip the bool
	if options.SkipTLSVerify != nil {
		params.Del("SkipTLSVerify")
		params.Set("tlsVerify", strconv.FormatBool(!options.GetSkipTLSVerify()))
	}
	params.Set("destination", destination)

	path := fmt.Sprintf("/images/%s/push", source)
	response, err := conn.DoRequest(nil, http.MethodPost, path, params, header)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	return response.Process(err)
}

func authHeader(identityToken string) (map[string]string, error) {
	var (
		content string
		err     error
	)
	content, err = encodeSingleAuthConfig(types.DockerAuthConfig{IdentityToken: identityToken})
	if err != nil {
		return nil, err
	}

	if len(content) > 0 {
		return map[string]string{string(XRegistryAuthHeader): content}, nil
	}
	return nil, nil
}

// encodeSingleAuthConfig serializes the auth configuration as a base64 encoded JSON payload.
func encodeSingleAuthConfig(authConfig types.DockerAuthConfig) (string, error) {
	conf := imageAuthToDockerAuth(authConfig)
	buf, err := json.Marshal(conf)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

func imageAuthToDockerAuth(authConfig types.DockerAuthConfig) dockerAPITypes.AuthConfig {
	return dockerAPITypes.AuthConfig{
		Username:      authConfig.Username,
		Password:      authConfig.Password,
		IdentityToken: authConfig.IdentityToken,
	}
}
