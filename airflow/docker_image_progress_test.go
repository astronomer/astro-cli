package airflow

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/airflow/mocks"
)

// A push draws its progress where ProgressTo says, stdout otherwise: a
// command whose stdout carries a result sends it to stderr.
func (s *Suite) TestDockerImagePushProgressGoesWhereItIsSent() {
	prevDisplay, prevExec := displayJSONMessagesToStream, cmdExec
	defer func() {
		getDockerClient = s.origGetDockerClient
		displayJSONMessagesToStream, cmdExec = prevDisplay, prevExec
	}()
	for _, c := range []struct {
		name string
		to   io.Writer
	}{
		{"by default, stdout", nil},
		{"after ProgressTo, its writer", new(bytes.Buffer)},
	} {
		s.Run(c.name, func() {
			handler := DockerImage{imageName: "testing"}
			if c.to != nil {
				handler.ProgressTo(c.to)
			}
			mockClient := new(mocks.DockerRegistryAPI)
			mockClient.On("NegotiateAPIVersion", context.Background()).Once()
			mockClient.On("ImagePush", context.Background(), "test", mock.Anything).Return(io.NopCloser(strings.NewReader("{}")), nil).Once()
			getDockerClient = func() (client.APIClient, error) { return mockClient, nil }
			cmdExec = func(string, io.Writer, io.Writer, ...string) error { return nil }
			var drewTo io.Writer
			displayJSONMessagesToStream = func(_ io.ReadCloser, w io.Writer, _ func(jsonmessage.JSONMessage)) error {
				drewTo = w
				return nil
			}

			_, err := handler.Push("test", "", "", false)
			s.NoError(err)
			if c.to == nil {
				s.Equal(os.Stdout, drewTo)
			} else {
				s.Same(c.to, drewTo)
			}
		})
	}
}
