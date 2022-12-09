package software

import (
	"bytes"
	"path/filepath"
	"regexp"
	"text/template"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/software/include"
	"github.com/astronomer/astro-cli/software/types"
	"github.com/pkg/errors"
)

var (
	acmCertArn               string
	emailNoReply             string
	privateLoadBalancer      bool
	privateRegistryRepo      string
	isAws                    bool
	isGcloud                 bool
	isAzure                  bool
	baseDomain               string
	privateCa                bool
	disableNginx             bool
	enableEmail              bool
	enableManualReleaseNames bool
)

func initAstroValues(airgapped bool, valuesFilepath string) error {
	cfg := types.ValuesConfig{
		AcmCertArn:               acmCertArn,
		Airgapped:                airgapped,
		Aws:                      isAws,
		Azure:                    isAzure,
		Gcloud:                   isGcloud,
		BaseDomain:               baseDomain,
		DisableNginx:             disableNginx,
		EmailNoReply:             emailNoReply,
		EnableEmail:              enableEmail,
		EnableManualReleaseNames: enableManualReleaseNames,
		EnablePublicSignups:      !enableEmail,
		PrivateCA:                privateCa,
		PrivateLoadBalancer:      privateLoadBalancer,
		PrivateRegistryRepo:      privateRegistryRepo,
	}

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
	valuesFilepath := filepath.Join(config.WorkingPath, "astro_values.yaml")

	if environment == "" {
		environment = input.Text("Enter the environment you are installing Astronomer in: (aws, azure, gcloud, other) ")
	}
	isAws = environment == "aws"
	isGcloud = environment == "gcloud"
	isAzure = environment == "azure"

	baseDomain = input.Text("Enter the base domain for Astronomer: ")
	privateCa, _ = input.Confirm("Is the SSl certificate signed by a private CA?")
	disableNginx, _ = input.Confirm("Disable built in Nginx ingress controller?")
	enableEmail, _ = input.Confirm("Enable email confirmations and alerts?")
	enableManualReleaseNames, _ = input.Confirm("Enable manual release names?")

	if !disableNginx {
		privateLoadBalancer, _ = input.Confirm("Use a private load balancer?")
	}

	if isAws && !disableNginx {
		acmCertArn = input.Text("Enter the ACM certificate ARN: ")
	}

	if enableEmail {
		emailNoReply = input.Text("Enter the no-reply email address: ")
	}

	if airgapped {
		privateRegistryRepo = input.Text("Enter the private docker registry repo for all images: ")
	}

	return initAstroValues(airgapped, valuesFilepath)
}
