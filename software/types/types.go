package types

type ValuesConfig struct {
	AcmCertArn                       string
	Airgapped                        bool
	AuthClientId                     string
	AuthClientSecretName             string
	AuthDiscoveryUrl                 string
	AuthProvider                     string
	AuthProviderName                 string
	Aws                              bool
	Azure                            bool
	BaseDomain                       string
	DisableNginx                     bool
	EmailNoReply                     string
	EnableEmail                      bool
	EnableManualReleaseNames         bool
	EnablePublicSignups              bool
	EnableRegistryBackend            bool
	NamespacePools                   NamespacePoolsConfig
	Gcloud                           bool
	PrivateCA                        bool
	PrivateLoadBalancer              bool
	PrivateRegistryRepo              string
	RegistryBackendAzureAccountKey   string
	RegistryBackendAzureAccountName  string
	RegistryBackendAzureContainer    string
	RegistryBackendBucket            string
	RegistryBackendProvider          string
	RegistryBackendS3AccessKeyId     string
	RegistryBackendS3EnableEncrypt   bool
	RegistryBackendS3KmsKey          string
	RegistryBackendS3Region          string
	RegistryBackendS3RegionEndpoint  string
	RegistryBackendS3SecretAccessKey string
	Secrets                          []KubernetesEnvironmentSecret
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
