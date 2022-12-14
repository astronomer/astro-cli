package software

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/prompt"
	"github.com/astronomer/astro-cli/software/include"
	"github.com/astronomer/astro-cli/software/types"
	"github.com/pkg/errors"
)

var (
	cfg = types.ValuesConfig{}
)

func initAstroValues(valuesFilepath string) error {
	tmpl, err := template.New("yml").Parse(include.Helmvalues)
	if err != nil {
		return errors.Wrap(err, "failed to generate values file")
	}

	buff := new(bytes.Buffer)
	if err := tmpl.Execute(buff, cfg); err != nil {
		return errors.Wrap(err, "failed to generate values file")
	}

	multipleNewline := regexp.MustCompile(`\n\s*\n`)
	astroValues := multipleNewline.ReplaceAllString(buff.String(), "\n\n")

	if err := fileutil.WriteStringToFile(valuesFilepath, astroValues); err != nil {
		return errors.Wrapf(err, "failed to create file '%s'", valuesFilepath)
	}

	return nil
}

func Init(environment string, airgapped bool) error {
	var err error
	valuesFilepath := filepath.Join(config.WorkingPath, "astro_values.yaml")

	if environment == "" {
		envContent := prompt.Content{
			ErrorMsg: "Please provide an environment",
			Label:    "What environment is Astronomer being installed in",
		}
		envItems := []string{"aws", "gcloud", "azure", "other"}
		if environment, err = prompt.GetSelect(envContent, envItems); err != nil {
			return errors.Wrap(err, envContent.ErrorMsg)
		}
	}
	cfg.Aws = environment == "aws"
	cfg.Gcloud = environment == "gcloud"
	cfg.Azure = environment == "azure"
	cfg.Airgapped = airgapped

	cfg.BaseDomain, _ = prompt.GetInput(prompt.Content{Label: "What is the base domain for Astronomer"}, false, nil)
	cfg.PrivateCA, _ = prompt.GetConfirm(prompt.Content{Label: "Is the SSl certificate signed by a private CA"})
	cfg.DisableNginx, _ = prompt.GetConfirm(prompt.Content{Label: "Disable built in Nginx ingress controller"})
	cfg.EnableEmail, _ = prompt.GetConfirm(prompt.Content{Label: "Enable email confirmations and alerts"})
	cfg.EnableManualReleaseNames, _ = prompt.GetConfirm(prompt.Content{Label: "Enable manual release names"})

	if !cfg.DisableNginx {
		cfg.PrivateLoadBalancer, _ = prompt.GetConfirm(prompt.Content{Label: "Use a private load balancer"})
	}

	if cfg.Aws && !cfg.DisableNginx {
		cfg.AcmCertArn, _ = prompt.GetInput(prompt.Content{Label: "What is the ARN of the ACM certificate"}, false, nil)
	}

	if cfg.EnableEmail {
		cfg.EmailNoReply, _ = prompt.GetInput(prompt.Content{Label: "What is the no-reply email address"}, false, nil)
		cfg.Secrets = append(cfg.Secrets, types.KubernetesEnvironmentSecret{EnvName: "EMAIL__SMTP_URL", SecretName: "astronomer-smtp", SecretKey: "connection"})
	}

	if airgapped {
		cfg.PrivateRegistryRepo, _ = prompt.GetInput(prompt.Content{Label: "What is the private docker registry repo URL"}, false, nil)
	}

	cfg.AuthProvider, _ = prompt.GetSelect(prompt.Content{Label: "What authentication provider will be used"}, []string{"local", "github", "google", "oidc", "oauth"})

	if cfg.AuthProvider == "oidc" || cfg.AuthProvider == "oauth" {
		cfg.AuthProviderName, _ = prompt.GetInput(prompt.Content{Label: "What is the name of the authentication provider (i.e. microsoft, okta, etc.)"}, false, nil)
		cfg.AuthClientId, _ = prompt.GetInput(prompt.Content{Label: fmt.Sprintf("What is the %s client ID", cfg.AuthProvider)}, false, nil)
		cfg.AuthDiscoveryUrl, _ = prompt.GetInput(prompt.Content{Label: fmt.Sprintf("What is the %s discovery URL", cfg.AuthProvider)}, false, nil)
	}

	if cfg.AuthProvider == "oauth" {
		clientSecretName, _ := prompt.GetInput(prompt.Content{Label: "What is the oauth client secret Kubernetes secret name"}, false, nil)
		cfg.Secrets = append(cfg.Secrets, types.KubernetesEnvironmentSecret{EnvName: fmt.Sprintf("AUTH__OPENID_CONNECT__%s__CLIENT_SECRET", strings.ToUpper(cfg.AuthProviderName)), SecretName: clientSecretName, SecretKey: "client_secret"})
	}

	if cfg.EnableRegistryBackend, _ = prompt.GetConfirm(prompt.Content{Label: "Enable a custom registry backend"}); cfg.EnableRegistryBackend {
		cfg.RegistryBackendProvider, _ = prompt.GetSelect(prompt.Content{Label: "What registry backend provider will be used"}, []string{"s3", "gcs", "azure"})
		if cfg.RegistryBackendProvider == "azure" {
			cfg.RegistryBackendAzureAccountName, _ = prompt.GetInput(prompt.Content{Label: "What is the registry Azure container account name"}, false, nil)
			cfg.RegistryBackendAzureAccountKey, _ = prompt.GetInput(prompt.Content{Label: "What is the registry Azure container account key"}, false, nil)
			cfg.RegistryBackendAzureContainer, _ = prompt.GetInput(prompt.Content{Label: "What is the registry Azure container name"}, false, nil)
		} else if cfg.RegistryBackendProvider == "gcs" {
			cfg.RegistryBackendBucket, _ = prompt.GetInput(prompt.Content{Label: "What is the registry GCS bucket name"}, false, nil)
		} else {
			cfg.RegistryBackendBucket, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 bucket name"}, false, nil)
			cfg.RegistryBackendS3AccessKeyId, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 access key ID"}, false, nil)
			cfg.RegistryBackendS3SecretAccessKey, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 secret access key"}, false, nil)
			cfg.RegistryBackendS3Region, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 region"}, false, nil)
			cfg.RegistryBackendS3RegionEndpoint, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 region endpoint"}, false, nil)
			if cfg.RegistryBackendS3EnableEncrypt, _ = prompt.GetConfirm(prompt.Content{Label: "Enable registry S3 KMS encryption"}); cfg.RegistryBackendS3EnableEncrypt {
				cfg.RegistryBackendS3KmsKey, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 KMS key"}, false, nil)
			}
		}
	}

	if cfg.NamespacePools.Enabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable namespace pools configuration"}); cfg.NamespacePools.Enabled {
		cfg.NamespacePools.Create, _ = prompt.GetConfirm(prompt.Content{Label: "Create namespaces if they do not exist"})
		for name, _ := prompt.GetInput(prompt.Content{Label: "Enter a namespace name"}, false, nil); name != ""; name, _ = prompt.GetInput(prompt.Content{Label: "Enter a namespace name"}, false, nil) {
			cfg.NamespacePools.Names = append(cfg.NamespacePools.Names, name)
		}
	}

	return initAstroValues(valuesFilepath)
}
