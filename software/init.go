package software

import (
	"bytes"
	"encoding/base64"
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

	multipleNewline := regexp.MustCompile(`\n\n+`)
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
		envItems := []string{"aws", "google", "azure", "other"}
		if environment, err = prompt.GetSelect(envContent, envItems); err != nil {
			return errors.Wrap(err, envContent.ErrorMsg)
		}
	}
	cfg.Environment = types.EnvironmentConfig{
		Airgapped: airgapped,
		IsAws:     environment == "aws",
		IsAzure:   environment == "azure",
		IsGoogle:  environment == "google",
	}

	cfg.BaseDomain, _ = prompt.GetInput(prompt.Content{Label: "What is the base domain for Astronomer"}, false, nil)
	cfg.PrivateCA, _ = prompt.GetConfirm(prompt.Content{Label: "Is the SSl certificate signed by a private CA"})
	cfg.ManualReleaseNamesEnabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable manual release names"})

	if cfg.Email.Enabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable email confirmations and alerts"}); cfg.Email.Enabled {
		cfg.Email.NoReply, _ = prompt.GetInput(prompt.Content{Label: "What is the no-reply email address"}, false, nil)
		cfg.Email.SmtpUrl, _ = prompt.GetInput(prompt.Content{Label: "What is the SMTP URL (leave blank to use an `astronomer-smtp` secret)"}, false, nil)
		if cfg.Email.SmtpUrl == "" {
			cfg.Secrets = append(cfg.Secrets, types.KubernetesEnvironmentSecret{EnvName: "EMAIL__SMTP_URL", SecretName: "astronomer-smtp", SecretKey: "connection"})
		}
	}

	if cfg.ThirdPartyIngress.Enabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable third party ingress controller"}); cfg.ThirdPartyIngress.Enabled {
		cfg.ThirdPartyIngress.Provider, _ = prompt.GetSelect(prompt.Content{Label: "What is the third party ingress controller provider"}, []string{"nginx", "traefik", "contour", "other"})
		cfg.ThirdPartyIngress.IngressClassName, _ = prompt.GetInput(prompt.Content{Label: "What is the ingress class name"}, false, nil)
	}

	if !cfg.ThirdPartyIngress.Enabled {
		cfg.PrivateLoadBalancer, _ = prompt.GetConfirm(prompt.Content{Label: "Use a private load balancer"})
	}

	if cfg.Environment.IsAws && !cfg.ThirdPartyIngress.Enabled {
		cfg.AcmCertArn, _ = prompt.GetInput(prompt.Content{Label: "What is the ARN of the ACM certificate"}, false, nil)
	}

	cfg.Auth.Provider, _ = prompt.GetSelect(prompt.Content{Label: "What authentication provider will be used"}, []string{"local", "github", "google", "oidc", "oauth"})
	if cfg.Auth.Provider == "oidc" || cfg.Auth.Provider == "oauth" {
		cfg.Auth.ProviderName, _ = prompt.GetInput(prompt.Content{Label: "What is the name of the authentication provider (i.e. microsoft, okta, etc.)"}, false, nil)
		cfg.Auth.ClientId, _ = prompt.GetInput(prompt.Content{Label: fmt.Sprintf("What is the %s client ID", cfg.Auth.Provider)}, false, nil)
		cfg.Auth.DiscoveryUrl, _ = prompt.GetInput(prompt.Content{Label: fmt.Sprintf("What is the %s discovery URL", cfg.Auth.Provider)}, false, nil)
		if cfg.Auth.IdpGroupImportEnabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable IdP group import"}); cfg.Auth.IdpGroupImportEnabled {
			cfg.Auth.GroupsClaimName, _ = prompt.GetInput(prompt.Content{Label: "What is the groups claim name (leave blank if 'groups')"}, false, nil)
			cfg.Auth.DisableUserManagement, _ = prompt.GetConfirm(prompt.Content{Label: "Disable user management and only allow Team invites"})
		}
	}

	if cfg.Auth.Provider == "oauth" {
		clientSecretName, _ := prompt.GetInput(prompt.Content{Label: "What is the oauth client secret Kubernetes secret name"}, false, nil)
		cfg.Secrets = append(cfg.Secrets, types.KubernetesEnvironmentSecret{EnvName: fmt.Sprintf("AUTH__OPENID_CONNECT__%s__CLIENT_SECRET", strings.ToUpper(cfg.Auth.ProviderName)), SecretName: clientSecretName, SecretKey: "client_secret"})
	}

	if airgapped {
		cfg.Registry.PrivateRepositoryUrl, _ = prompt.GetInput(prompt.Content{Label: "What is the private docker registry repository URL for airgapped environment"}, false, nil)
		cfg.SelfHostedHelmRepo, _ = prompt.GetInput(prompt.Content{Label: "What is the self hosted astronomer helm repository URL for airgapped environment (leave blank to use commander built-in Airflow repo)"}, false, nil)
	}

	if cfg.Registry.CustomImageRepo.Enabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable custom image repository for Airflow images"}); cfg.Registry.CustomImageRepo.Enabled {
		cfg.Registry.CustomImageRepo.AirflowImageRepo, _ = prompt.GetInput(prompt.Content{Label: "What is the Airflow image repository URL"}, false, nil)
		cfg.Registry.CustomImageRepo.CredentialsSecretName, _ = prompt.GetInput(prompt.Content{Label: "What is the name of the docker registry Kubernetes secret"}, false, nil)
	} else {
		if cfg.Registry.Backend.Enabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable a registry backend for Airflow deployment images"}); cfg.Registry.Backend.Enabled {
			cfg.Registry.Backend.Provider, _ = prompt.GetSelect(prompt.Content{Label: "What registry backend provider will be used"}, []string{"s3", "gcs", "azure"})
			if cfg.Registry.Backend.Provider == "azure" {
				cfg.Registry.Backend.AzureAccountName, _ = prompt.GetInput(prompt.Content{Label: "What is the registry Azure container account name"}, false, nil)
				cfg.Registry.Backend.AzureAccountKey, _ = prompt.GetInput(prompt.Content{Label: "What is the registry Azure container account key"}, false, nil)
				cfg.Registry.Backend.AzureContainer, _ = prompt.GetInput(prompt.Content{Label: "What is the registry Azure container name"}, false, nil)
			} else if cfg.Registry.Backend.Provider == "gcs" {
				cfg.Registry.Backend.Bucket, _ = prompt.GetInput(prompt.Content{Label: "What is the registry GCS bucket name"}, false, nil)
			} else {
				cfg.Registry.Backend.Bucket, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 bucket name"}, false, nil)
				cfg.Registry.Backend.S3AccessKeyId, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 access key ID"}, false, nil)
				cfg.Registry.Backend.S3SecretAccessKey, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 secret access key"}, false, nil)
				cfg.Registry.Backend.S3Region, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 region"}, false, nil)
				cfg.Registry.Backend.S3RegionEndpoint, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 region endpoint"}, false, nil)
				if cfg.Registry.Backend.S3EncryptEnabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable registry S3 KMS encryption"}); cfg.Registry.Backend.S3EncryptEnabled {
					cfg.Registry.Backend.S3KmsKey, _ = prompt.GetInput(prompt.Content{Label: "What is the registry S3 KMS key"}, false, nil)
				}
			}
		}
	}

	if cfg.NamespacePools.Enabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable namespace pools configuration"}); cfg.NamespacePools.Enabled {
		cfg.NamespacePools.Create, _ = prompt.GetConfirm(prompt.Content{Label: "Create namespaces if they do not exist"})
		for name, _ := prompt.GetInput(prompt.Content{Label: "Enter a namespace name"}, false, nil); name != ""; name, _ = prompt.GetInput(prompt.Content{Label: "Enter a namespace name"}, false, nil) {
			cfg.NamespacePools.Names = append(cfg.NamespacePools.Names, name)
		}
	}

	if cfg.Logging.ExternalElasticsearch.Enabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable sending logs to external Elasticsearch"}); cfg.Logging.ExternalElasticsearch.Enabled {
		cfg.Logging.ExternalElasticsearch.HostUrl, _ = prompt.GetInput(prompt.Content{Label: "What is the Elasticsearch host URL"}, false, nil)
		if cfg.Logging.ExternalElasticsearch.SecretCredentials, _ = prompt.GetInput(prompt.Content{Label: "What are the Elasticsearch credentials [i.e username:password] (leave blank to use Kubernetes secret)"}, false, nil); cfg.Logging.ExternalElasticsearch.SecretCredentials != "" {
			cfg.Logging.ExternalElasticsearch.SecretCredentials = base64.StdEncoding.EncodeToString([]byte(cfg.Logging.ExternalElasticsearch.SecretCredentials))
		}
	} else if cfg.Logging.SidecarLoggingEnabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable sidecar logging"}); !cfg.Logging.SidecarLoggingEnabled {
		if cfg.Logging.S3Logs.Enabled, _ = prompt.GetConfirm(prompt.Content{Label: "Enable log forwarding to S3"}); cfg.Logging.S3Logs.Enabled {
			cfg.Logging.S3Logs.RoleArn, _ = prompt.GetInput(prompt.Content{Label: "What is the S3 logging role ARN"}, false, nil)
			cfg.Logging.S3Logs.S3Bucket, _ = prompt.GetInput(prompt.Content{Label: "What is the S3 logging bucket"}, false, nil)
			cfg.Logging.S3Logs.S3Region, _ = prompt.GetInput(prompt.Content{Label: "What is the S3 logging region"}, false, nil)
		}
	}

	return initAstroValues(valuesFilepath)
}
