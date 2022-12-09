package include

import "strings"

// Composeyml is the docker-compose template
var Helmvalues = strings.TrimSpace(`
global:
  baseDomain: {{ .BaseDomain }}  # Base domain for all subdomains exposed through ingress
  tlsSecret: astronomer-tls  # Name of secret containing TLS certificate
  nginxEnabled: {{ not .DisableNginx }}
  defaultDenyNetworkPolicy: false
{{if .PrivateCA}}
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
{{end}}
{{if .Azure}}
  # Enables using SSL connections to
  # encrypt client/server communication
  # between databases and the Astronomer platform.
  # If your database enforces SSL for connections,
  # change this value to true
  ssl:
    enabled: true
    mode: "prefer"
{{end}}
{{if .Airgapped}}
  privateRegistry:
    enabled: true
    repository: {{ .PrivateRegistryRepo }}
{{end}}
{{if not .DisableNginx}}
nginx:
  loadBalancerIP: ~  # IP address the nginx ingress should bind to
  privateLoadBalancer: {{ .PrivateLoadBalancer }}  # Set to 'true' when deploying to a private EKS cluster
  # Dict of arbitrary annotations to add to the nginx ingress.
  # For full configuration options, see https://docs.nginx.com/nginx-ingress-controller/configuration/ingress-resources/advanced-configuration-with-annotations/
  {{if .Aws -}}
  ingressAnnotations:
    service.beta.kubernetes.io/aws-load-balancer-type: nlb  # Change to 'elb' if your node group is private and doesn't utilize a NAT gateway
    service.beta.kubernetes.io/aws-load-balancer-ssl-cert: {{ .AcmCertArn }}
    service.beta.kubernetes.io/aws-load-balancer-backend-protocol: ssl
  {{else -}}
  ingressAnnotations: {}
  {{end -}}
{{end}}
astronomer:
  {{if .Airgapped -}}
  commander:
    airGapped:
      enabled: true
  {{end -}}
  houston:
    {{if .Airgapped -}}
    updateCheck: # There is a 2nd check for Astronomer platform updates but this is deprecated and not actively used. Therefore disable
      enabled: false
    updateAirflowCheck: # Configure URL for Airflow updates check
      url: http://astronomer-releases.astronomer.svc.cluster.local/astronomer-certified
    updateRuntimeCheck: # Configure URL for Airflow updates check
      url: http://astronomer-releases.astronomer.svc.cluster.local/astronomer-runtime
    {{end -}}
    config:
      publicSignups: {{ .EnablePublicSignups }}  # Users need to be invited to have access to Astronomer. Set to true otherwise
      emailConfirmation: {{ and (not .EnablePublicSignups) .EnableEmail }}  # Users get an email verification before accessing Astronomer
      deployments:
        {{if .Aws -}}
        serviceAccountAnnotationKey: eks.amazonaws.com/role-arn  # Flag to enable using IAM roles (don't enter a specific role)
        {{else if .Gcloud -}}
        serviceAccountAnnotationKey: iam.gke.io/gcp-service-account  # Flag to enable using IAM roles (don't enter a specific role)
        {{end -}}
        {{if .Airgapped -}}
        helm:
          runtimeImages:
            airflow:
              repository: {{ .PrivateRegistryRepo }}/astro-runtime
            flower:
              repository: {{ .PrivateRegistryRepo }}/astro-runtime
          airflow:
            defaultAirflowRepository: {{ .PrivateRegistryRepo }}/ap-airflow
            images:
              airflow:
                repository: {{ .PrivateRegistryRepo }}/ap-airflow
              statsd:
                repository: {{ .PrivateRegistryRepo }}/ap-statsd-exporter
              redis:
                repository: {{ .PrivateRegistryRepo }}/ap-redis
              pgbouncer:
                repository: {{ .PrivateRegistryRepo }}/ap-pgbouncer
              pgbouncerExporter:
                repository: {{ .PrivateRegistryRepo }}/ap-pgbouncer-exporter
        {{end -}}
        manualReleaseNames: {{ .EnableManualReleaseNames }}  # Allows you to set your release names
      email:
        enabled: {{ .EnableEmail }}
        reply: {{ .EmailNoReply }}  # Emails will be sent from this address
      auth:
        github:
          enabled: true  # Lets users authenticate with Github
        local:
          enabled: false  # Disables logging in with just a username and password
        openidConnect:
          google:
            enabled: true  # Lets users authenticate with Google
    {{if .EnableEmail -}}
    secret:
    # Reference to the Kubernetes secret for SMTP credentials. Can be removed if email is not used.
    - envName: "EMAIL__SMTP_URL"
      secretName: "astronomer-smtp"
      secretKey: "connection"
{{end}}
`)
