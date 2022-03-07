package airflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/buildah"
	"github.com/containers/buildah/imagebuildah"
	"github.com/containers/podman/v3/pkg/bindings/images"
	"github.com/containers/podman/v3/pkg/domain/entities"
	"github.com/pkg/errors"
)

type PodmanImage struct {
	imageName  string
	podmanBind PodmanBind
	conn       context.Context
}

func PodmanImageInit(conn context.Context, image string, bind PodmanBind) (*PodmanImage, error) {
	if bind == nil {
		bind = &PodmanBinder{}
	}
	if conn == nil {
		var err error
		conn, err = getConn(context.TODO(), bind)
		if err != nil {
			return nil, err
		}
	}

	// We use latest and keep this tag around after deployments to keep subsequent deploys quick
	return &PodmanImage{imageName: image, conn: conn, podmanBind: bind}, nil
}

func (p *PodmanImage) Build(path string) error {
	err := os.Chdir(path)
	if err != nil {
		return err
	}

	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}
	buildahOpts := imagebuildah.BuildOptions{
		NoCache:                 false,
		RemoveIntermediateCtrs:  true,
		ForceRmIntermediateCtrs: true,
		ContextDirectory:        projectDir,
		Output:                  imageName(p.imageName, "latest"),
		Quiet:                   false,
		CommonBuildOpts:         &buildah.CommonBuildOptions{},
		OutputFormat:            buildah.Dockerv2ImageManifest,
	}
	options := entities.BuildOptions{BuildOptions: buildahOpts}
	_, err = p.podmanBind.Build(p.conn, []string{filepath.Join(projectDir, "Dockerfile")}, options)
	if err != nil {
		// fmt this
		return err
	}
	return nil
}

func (p *PodmanImage) Push(cloudDomain, token, remoteImageTag string) error {
	registry := "registry." + cloudDomain
	remoteImage := fmt.Sprintf("%s/%s", registry, imageName(p.imageName, remoteImageTag))

	err := p.podmanBind.Tag(p.conn, imageName(p.imageName, "latest"), remoteImageTag, fmt.Sprintf("%s/%s", registry, p.imageName), nil)
	if err != nil {
		return errors.Wrapf(err, "command 'podman tag %s %s' failed", p.imageName, remoteImage)
	}
	options := new(images.PushOptions)
	if err := p.podmanBind.Push(p.conn, p.imageName, remoteImage, options); err != nil {
		return errors.Wrapf(err, "Error pushing %s image to %s", p.imageName, registry)
	}

	err = p.podmanBind.Untag(p.conn, imageName(p.imageName, "latest"), remoteImageTag, fmt.Sprintf("%s/%s", registry, p.imageName), nil)
	if err != nil {
		return errors.Wrapf(err, "command 'podman untag %s' failed", remoteImage)
	}
	return nil
}

func (p *PodmanImage) GetImageLabels() (map[string]string, error) {
	imageReport, err := p.podmanBind.GetImage(p.conn, imageName(p.imageName, "latest"), nil)
	if err != nil {
		var labels map[string]string
		return labels, err
	}
	return imageReport.Labels, nil
}
