package airflow

import (
	"context"

	podmanImages "github.com/astronomer/astro-cli/pkg/podman"
	"github.com/containers/common/pkg/auth"
	"github.com/containers/image/v5/types"
	"github.com/containers/podman/v3/pkg/api/handlers"
	"github.com/containers/podman/v3/pkg/bindings"
	"github.com/containers/podman/v3/pkg/bindings/containers"
	"github.com/containers/podman/v3/pkg/bindings/images"
	"github.com/containers/podman/v3/pkg/bindings/play"
	"github.com/containers/podman/v3/pkg/bindings/pods"
	"github.com/containers/podman/v3/pkg/domain/entities"
)

type PodmanBinder struct{}

type PodmanBind interface {
	// Methods to handle container operations
	NewConnection(ctx context.Context, uri string) (context.Context, error)
	ExecCreate(ctx context.Context, nameOrID string, config *handlers.ExecCreateConfig) (string, error)
	ExecStartAndAttach(ctx context.Context, sessionID string, options *containers.ExecStartAndAttachOptions) error
	List(ctx context.Context, options *containers.ListOptions) ([]entities.ListContainer, error)
	Logs(ctx context.Context, nameOrID string, options *containers.LogOptions, stdoutChan, stderrChan chan string) error
	Kube(ctx context.Context, path string, options *play.KubeOptions) (*entities.PlayKubeReport, error)
	KubeDown(ctx context.Context, path string) (*entities.PlayKubeReport, error)
	Stop(ctx context.Context, nameOrID string, options *pods.StopOptions) (*entities.PodStopReport, error)
	Exists(ctx context.Context, nameOrID string, options *pods.ExistsOptions) (bool, error)
	Start(ctx context.Context, nameOrID string, options *pods.StartOptions) (*entities.PodStartReport, error)
	// Methods to handle image operations
	Build(ctx context.Context, containerFiles []string, options entities.BuildOptions) (*entities.BuildReport, error)
	Tag(ctx context.Context, nameOrID, tag, repo string, options *images.TagOptions) error
	Push(ctx context.Context, source, destination string, options *podmanImages.PushOptions) error
	Untag(ctx context.Context, nameOrID, tag, repo string, options *images.UntagOptions) error
	// Method to handle registry operations
	Login(ctx context.Context, systemContext *types.SystemContext, opts *auth.LoginOptions, args []string) error
}

func (p *PodmanBinder) NewConnection(ctx context.Context, uri string) (context.Context, error) {
	return bindings.NewConnection(ctx, uri)
}

func (p *PodmanBinder) ExecCreate(ctx context.Context, nameOrID string, config *handlers.ExecCreateConfig) (string, error) {
	return containers.ExecCreate(ctx, nameOrID, config)
}

func (p *PodmanBinder) ExecStartAndAttach(ctx context.Context, sessionID string, options *containers.ExecStartAndAttachOptions) error {
	return containers.ExecStartAndAttach(ctx, sessionID, options)
}

func (p *PodmanBinder) List(ctx context.Context, options *containers.ListOptions) ([]entities.ListContainer, error) {
	return containers.List(ctx, options)
}

func (p *PodmanBinder) Logs(ctx context.Context, nameOrID string, options *containers.LogOptions, stdoutChan, stderrChan chan string) error {
	return containers.Logs(ctx, nameOrID, options, stdoutChan, stderrChan)
}

func (p *PodmanBinder) Kube(ctx context.Context, path string, options *play.KubeOptions) (*entities.PlayKubeReport, error) {
	return play.Kube(ctx, path, options)
}

func (p *PodmanBinder) KubeDown(ctx context.Context, path string) (*entities.PlayKubeReport, error) {
	return play.KubeDown(ctx, path)
}

func (p *PodmanBinder) Stop(ctx context.Context, nameOrID string, options *pods.StopOptions) (*entities.PodStopReport, error) {
	return pods.Stop(ctx, nameOrID, options)
}

func (p *PodmanBinder) Exists(ctx context.Context, nameOrID string, options *pods.ExistsOptions) (bool, error) {
	return pods.Exists(ctx, nameOrID, options)
}

func (p *PodmanBinder) Start(ctx context.Context, nameOrID string, options *pods.StartOptions) (*entities.PodStartReport, error) {
	return pods.Start(ctx, nameOrID, options)
}

func (p *PodmanBinder) Build(ctx context.Context, containerFiles []string, options entities.BuildOptions) (*entities.BuildReport, error) { //nolint:gocritic
	return images.Build(ctx, containerFiles, options)
}

func (p *PodmanBinder) Tag(ctx context.Context, nameOrID, tag, repo string, options *images.TagOptions) error {
	return images.Tag(ctx, nameOrID, tag, repo, options)
}

func (p *PodmanBinder) Push(ctx context.Context, source, destination string, options *podmanImages.PushOptions) error {
	return podmanImages.Push(ctx, source, destination, options)
}

func (p *PodmanBinder) Untag(ctx context.Context, nameOrID, tag, repo string, options *images.UntagOptions) error {
	return images.Untag(ctx, nameOrID, tag, repo, options)
}

func (p *PodmanBinder) Login(ctx context.Context, systemContext *types.SystemContext, opts *auth.LoginOptions, args []string) error {
	return auth.Login(ctx, systemContext, opts, args)
}
