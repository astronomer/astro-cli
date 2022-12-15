package include

import "strings"

// Composeyml is the docker-compose template
var Helmvalues = strings.TrimSpace(`
global:
  baseDomain: {{ .BaseDomain }}  # Base domain for all subdomains exposed through ingress
  tlsSecret: astronomer-tls  # Name of secret containing TLS certificate
  defaultDenyNetworkPolicy: false
{{- if .ThirdPartyIngress.Enabled}}
  nginxEnabled: false
{{- end}}
{{- if .Logging.ExternalElasticsearch.Enabled}}
  fluentdEnabled: true
  customLogging:
    enabled: true
    scheme: https
    # host endpoint copied from elasticsearch console with https
    # and port number removed.
    host: {{ .Logging.ExternalElasticsearch.HostUrl}}
    port: "9243"
  {{- if not (eq .Logging.ExternalElasticsearch.SecretCredentials "")}}
    # encoded credentials
    secret: "{{ .Logging.ExternalElasticsearch.SecretCredentials }}"
  {{- else}}
    # kubernetes secret containing credentials
    # Example command: ` + "`kubectl create secret generic elasticcreds --from-literal elastic=<username>:<password> --namespace=<your-platform-namespace>`" + `
    secretName: elasticcreds
  {{- end}}
{{- else if .Logging.SidecarLoggingEnabled}}
  fluentdEnabled: false
  loggingSidecar:
    enabled: true
    name: sidecar-log-consumer
    # needed to prevent zombie deployment worker pods when using KubernetesExecutor
    terminationEndpoint: http://localhost:8000/quitquitquit
{{- end}}
{{- if and .NamespacePools.Enabled (not .NamespacePools.Create)}}
  manualNamespaceNamesEnabled: true
  clusterRoles: false
{{- end}}
{{- if not (eq .SelfHostedHelmRepo "")}}
  helmRepo: "{{ .SelfHostedHelmRepo }}"
{{- end}}
{{- if .PrivateCA}}
  # Enable privateCaCerts only if your enterprise security team
  # generated a certificate from a private certificate authority.
  # Create a generic secret for each cert, and add it to the list below.
  # Each secret must have a data entry for 'cert.pem'
  # Example command: ` + "`kubectl create secret generic private-root-ca --from-file=cert.pem=./<your-certificate-filepath>`" + `
  privateCaCerts:
  - private-root-ca
  # Enable privateCaCertsAddToHost only when your nodes do not already
  # include the private CA in their docker trust store.
  # Most enterprises already have this configured,
  # and in that case 'enabled' should be false.
  privateCaCertsAddToHost:
    enabled: true
    hostDirectory: /etc/docker/certs.d
{{- end}}
{{- if .ThirdPartyIngress.Enabled}}
  extraAnnotations:
    # if not using Astronomers built-in ingress controller, you MUST
    # explicitly set kubernetes.io/ingress.class here
    kubernetes.io/ingress.class: {{ .ThirdPartyIngress.IngressClassName }}
  {{- if eq .ThirdPartyIngress.Provider "nginx"}}
    nginx.ingress.kubernetes.io/proxy-body-size: 0
  {{- else if eq .ThirdPartyIngress.Provider "traefik"}}
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
  {{- end}}
  authSidecar:
    enabled: true
  {{- if .Environment.Airgapped}}
    repository: {{ .Registry.PrivateRepositoryUrl }}/nginx-unprivileged
  {{- else}}
    repository: nginxinc/nginx-unprivileged
  {{- end}}
    tag: stable
{{- end}}
{{- if .Environment.IsAzure}}
  # Enables using SSL connections to
  # encrypt client/server communication
  # between databases and the Astronomer platform.
  # If your database enforces SSL for connections,
  # change this value to true
  ssl:
    enabled: true
    mode: "prefer"
{{- end}}
{{- if .Environment.Airgapped}}
  privateRegistry:
    enabled: true
    repository: {{ .Registry.PrivateRepositoryUrl }}
{{- end}}
{{- if .NamespacePools.Create}}
  features:
    namespacePools:
      # if this is false, everything in this section can be ignored. default should be false
      enabled: {{ .NamespacePools.Enabled }}
      namespaces:
        # automatically creates namespace, role and rolebinding for commander if set to true
        create: {{ .NamespacePools.Create }}
      {{- if .NamespacePools.Names}}
        # this needs to be populated (something other than null) if global.features.namespacePools.enabled is true
        names:
        {{- range .NamespacePools.Names}}
          - {{ . }}
        {{- end}}
      {{- end}}
{{- end}}
{{if not .ThirdPartyIngress.Enabled}}
nginx:
  loadBalancerIP: ~  # IP address the nginx ingress should bind to
  privateLoadBalancer: {{ .PrivateLoadBalancer }}  # Set to 'true' when deploying to a private EKS cluster
  # Dict of arbitrary annotations to add to the nginx ingress.
  # For full configuration options, see https://docs.nginx.com/nginx-ingress-controller/configuration/ingress-resources/advanced-configuration-with-annotations/
{{- if .Environment.IsAws}}
  ingressAnnotations:
    service.beta.kubernetes.io/aws-load-balancer-type: nlb  # Change to 'elb' if your node group is private and doesn't utilize a NAT gateway
    service.beta.kubernetes.io/aws-load-balancer-ssl-cert: {{ .AcmCertArn }}
    service.beta.kubernetes.io/aws-load-balancer-backend-protocol: ssl
{{- else}}
  ingressAnnotations: {}
{{- end}}
{{end}}
astronomer:
  commander:
    airGapped:
      enabled: {{ and .Environment.Airgapped (eq .SelfHostedHelmRepo "")}} # disable if using self hosted helm repository or not airgapped
  {{- if and .NamespacePools.Enabled (not .NamespacePools.Create)}}
    env:
      - name: "COMMANDER_MANUAL_NAMESPACE_NAMES"
        value: true
  {{- end}}
  houston:
  {{- if .Environment.Airgapped}}
    updateCheck: # There is a 2nd check for Astronomer platform updates but this is deprecated and not actively used. Therefore disable
      enabled: false
    updateAirflowCheck: # Configure URL for Airflow updates check
      url: http://astronomer-releases.astronomer.svc.cluster.local/astronomer-certified
    updateRuntimeCheck: # Configure URL for Airflow updates check
      url: http://astronomer-releases.astronomer.svc.cluster.local/astronomer-runtime
  {{- end}}
    config:
      publicSignups: {{ .PublicSignupsEnabled }}  # Users need to be invited to have access to Astronomer. Set to true otherwise
      emailConfirmation: {{ and (not .PublicSignupsEnabled) .Email.Enabled }}  # Users get an email verification before accessing Astronomer
      deployments:
      {{- if .Environment.IsAws}}
        serviceAccountAnnotationKey: eks.amazonaws.com/role-arn  # Flag to enable using IAM roles (don't enter a specific role)
      {{- else if .Environment.IsGoogle}}
        serviceAccountAnnotationKey: iam.gke.io/gcp-service-account  # Flag to enable using IAM roles (don't enter a specific role)
      {{- end}}
      {{- if .Environment.Airgapped}}
        helm:
          runtimeImages:
          {{- if .Registry.CustomImageRepo.Enabled}}
            airflow:
              repository: {{ .Registry.CustomImageRepo.AirflowImageRepo }}
            flower:
              repository: {{ .Registry.CustomImageRepo.AirflowImageRepo }}
          {{- else}}
            airflow:
              repository: {{ .Registry.PrivateRepositoryUrl }}/astro-runtime
            flower:
              repository: {{ .Registry.PrivateRepositoryUrl }}/astro-runtime
          {{- end}}
          airflow:
          {{- if .Registry.CustomImageRepo.Enabled}}
            defaultAirflowRepository: {{ .Registry.CustomImageRepo.AirflowImageRepo }}
          {{- else}}
            defaultAirflowRepository: {{ .Registry.PrivateRepositoryUrl }}/ap-airflow
          {{- end}}
            images:
              airflow:
              {{- if .Registry.CustomImageRepo.Enabled}}
                repository: {{ .Registry.CustomImageRepo.AirflowImageRepo }}
              {{- else}}
                repository: {{ .Registry.PrivateRepositoryUrl }}/ap-airflow
              {{- end}}
              statsd:
                repository: {{ .Registry.PrivateRepositoryUrl }}/ap-statsd-exporter
              redis:
                repository: {{ .Registry.PrivateRepositoryUrl }}/ap-redis
              pgbouncer:
                repository: {{ .Registry.PrivateRepositoryUrl }}/ap-pgbouncer
              pgbouncerExporter:
                repository: {{ .Registry.PrivateRepositoryUrl }}/ap-pgbouncer-exporter
      {{- end}}
      {{- if and .NamespacePools.Enabled (not .NamespacePools.Create)}}
        # Enable manual namespace names
        manualNamespaceNames: true
        # Pre-created namespace names
        preCreatedNamespaces:
        {{- range .NamespacePools.Names}}
          - name: {{ . }}
        {{- end}}
        # Allows users to immediately reuse a pre-created namespace by hard deleting the associated Deployment
        # If set to false, you'll need to wait until a cron job runs before the Deployment record is deleted and the namespace is added back to the pool
        hardDeleteDeployment: true
      {{- end}}
      {{- if .Registry.CustomImageRepo.Enabled}}
        enableUpdateDeploymentImageEndpoint: true
      {{- end}}
        manualReleaseNames: {{ .ManualReleaseNamesEnabled }}  # Allows you to set your release names
      email:
        enabled: {{ .Email.Enabled }}
        reply: "{{ .Email.NoReply }}"  # Emails will be sent from this address
      {{- if not (eq .Email.SmtpUrl "")}}
        smtpUrl: "{{ .Email.SmtpUrl }}"
      {{- end}}
    {{- if .Auth.DisableUserManagement}}
      userManagement:
        enabled: false
    {{- end}}
      auth:
        github:
          enabled: {{ eq .Auth.Provider "github" }}  # Lets users authenticate with Github
        local:
          enabled: {{ eq .Auth.Provider "local" }}  # Disables logging in with just a username and password
        openidConnect:
        {{- if eq .Auth.Provider "oauth"}}
          flow: "code"
        {{- end}}
        {{- if or (eq .Auth.Provider "oidc") (eq .Auth.Provider "oauth")}}
        {{- if .Auth.IdpGroupImportEnabled}}
          idpGroupsImportEnabled: true
        {{- end}}
          {{ .Auth.ProviderName }}:
            enabled: true
            client_id: {{ .Auth.ClientId }}
            discoveryUrl: {{ .Auth.DiscoveryUrl }}
          {{- if not (eq .Auth.GroupsClaimName "")}}
            claimsMapping: {{ .Auth.GroupsClaimName }}
          {{- end}}
        {{- end}}
          google:
            enabled: {{ eq .Auth.Provider "google" }}  # Lets users authenticate with Google
  {{- if .Secrets}}
    secret:
    {{- range .Secrets}}
      - envName: {{ .EnvName }}
        secretName: {{ .SecretName }}
        secretKey: {{ .SecretKey }}
    {{- end}}
  {{- end}}
  registry:
    protectedRegistry.CustomImageRepo:
      enabled: {{ .Registry.CustomImageRepo.Enabled }}
    {{- if and .Registry.CustomImageRepo.Enabled .Environment.Airgapped}}
      baseRegistry:
        enabled: true
        host: {{ .Registry.CustomImageRepo.AirflowImageRepo }}
        secretName: {{ .Registry.CustomImageRepo.CredentialsSecretName }}
    {{- end}}
    {{- if .Registry.CustomImageRepo.Enabled}}
      updateRegistry:
        enabled: true
        host: {{ .Registry.CustomImageRepo.AirflowImageRepo }}
        secretName: {{ .Registry.CustomImageRepo.CredentialsSecretName }}
    {{- end}}
  {{- if .Registry.Backend.Enabled}}
    {{ .Registry.Backend.Provider }}:
      enabled: true
    {{- if eq .Registry.Backend.Provider "gcs"}}
      bucket: {{ .Registry.Backend.Bucket }}
    {{- else if eq .Registry.Backend.Provider "s3"}}
      bucket: {{ .Registry.Backend.Bucket }}
    {{- if .Registry.Backend.S3AccessKeyId}}
      accesskey: {{ .Registry.Backend.S3AccessKeyId }}
      secretkey: {{ .Registry.Backend.S3SecretAccessKey }}
    {{- end}}
      region: {{ .Registry.Backend.S3Region }}
    {{- if .Registry.Backend.S3RegionEndpoint}}
      regionendpoint: {{ .Registry.Backend.S3RegionEndpoint }}
    {{- end}}
    {{- if .Registry.Backend.S3EncryptEnabled}}
      encrypt: {{ .Registry.Backend.S3EncryptEnabled }}
      keyid: {{ .Registry.Backend.S3KmsKey }}
    {{- end}}
  {{- else}}
      accountname: {{ .Registry.Backend.AzureAccountName }}
      accountkey: {{ .Registry.Backend.AzureAccountKey }}
      container: {{ .Registry.Backend.AzureContainer }}
      realm: core.windows.net
  {{- end}}
{{- end}}
{{- if eq .ThirdPartyIngress.Provider "contour"}}
  extraObjects:
    - apiVersion: projectcontour.io/v1
      kind: HTTPProxy
      metadata:
        name: houston
        annotations:
          kubernetes.io/ingress.class: {{ .ThirdPartyIngress.IngressClassName }}
      spec:
        virtualhost:
          fqdn: houston.{{ .BaseDomain }}
          tls:
            secretName: astronomer-tls
        routes:
          - conditions:
              - prefix: /ws
            enableWebsockets: true
            services:
              - name: astronomer-houston
                port: 8871
{{- end}}
{{if .Logging.S3Logs.Enabled}}
fluentd:
  s3:
    enabled: true
    role_arn: {{ .Logging.S3Logs.RoleArn }}
    role_session_name: astronomer-logs
    s3_bucket: {{ .Logging.S3Logs.S3Bucket }}
    s3_region: {{ .Logging.S3Logs.S3Region }}
{{end}}
{{- if .Logging.ExternalElasticsearch.Enabled}}
tags:
  logging: false
{{end}}
`)
