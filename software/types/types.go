package types

type ValuesConfig struct {
	AcmCertArn                string
	Auth                      AuthConfig
	BaseDomain                string
	Email                     EmailConfig
	Environment               EnvironmentConfig
	ManualReleaseNamesEnabled bool
	NamespacePools            NamespacePoolsConfig
	PrivateCA                 bool
	PrivateLoadBalancer       bool
	PublicSignupsEnabled      bool
	Registry                  RegistryConfig
	Secrets                   []KubernetesEnvironmentSecret
	SelfHostedHelmRepo        string
	ThirdPartyIngress         ThirdPartyIngressConfig
}

type AuthConfig struct {
	ClientId         string
	ClientSecretName string
	DiscoveryUrl     string
	Provider         string
	ProviderName     string
}

type CustomImageRepoConfig struct {
	Enabled               bool
	AirflowImageRepo      string
	CredentialsSecretName string
}

type EmailConfig struct {
	Enabled bool
	NoReply string
	SmtpUrl string
}

type EnvironmentConfig struct {
	Airgapped     bool
	IsAws         bool
	IsAzure       bool
	IsGoogleCloud bool
}

type KubernetesEnvironmentSecret struct {
	EnvName    string
	SecretName string
	SecretKey  string
}

type NamespacePoolsConfig struct {
	Enabled bool
	Create  bool
	Names   []string
}

type RegistryConfig struct {
	PrivateRepositoryUrl string
	CustomImageRepo      CustomImageRepoConfig
	Backend              RegistryBackendConfig
}

type RegistryBackendConfig struct {
	Enabled           bool
	AzureAccountKey   string
	AzureAccountName  string
	AzureContainer    string
	Bucket            string
	Provider          string
	S3AccessKeyId     string
	S3EncryptEnabled  bool
	S3KmsKey          string
	S3Region          string
	S3RegionEndpoint  string
	S3SecretAccessKey string
}

type ThirdPartyIngressConfig struct {
	Enabled          bool
	Provider         string
	IngressClassName string
}
