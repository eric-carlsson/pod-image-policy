# pod-image-policy

A Kubernetes admission controller that validates and mutates container image references in pods.

## Features

- Validate image segments with allow/deny/warn actions and custom messages.
- Rewrite image segments using regex capture groups and replacement templates with optional messages.

### Examples

- Reject images with `latest` tag:

  ```yaml
  validate:
    rules:
      - match:
          tag: "^latest$"
        action: deny
        message: "'latest' image tags are not allowed"
  ```

- Warn when using release candidates:

  ```yaml
  validate:
    rules:
      - match:
          tag: ".*-rc.*"
        action: warn
        message: "prefer using stable releases over release candidates"
  ```

- Rewrite all public Docker Hub images to use a private mirror: `docker.io/library/<image>` (or `<image>`) &rarr; `registry.private/mirror/library/<image>`:

  ```yaml
  mutate:
    rules:
      - match:
          registry: "^docker\\.io$"
          repository: "^library/(.*)$"
        replace:
          registry: "registry.private"
          repository: "mirror/library/${1}"
  ```

- Rewrite specific images that have been moved to new repositories: `registry.io/team/app:v1` &rarr; `registry.io/project/app:v1`:

  ```yaml
  mutate:
    rules:
      - match:
          registry: "^registry\\.io$"
          repository: "^team/(.*)$"
        replace:
          repository: "project/${1}"
        message: "'team/' repositories have been moved to 'project/'"
  ```

## Deploy

A Helm chart is available in [helm/pod-image-policy](helm/pod-image-policy).

A TLS certificate is required to enable communication between the API server and the webhook. By default, the chart is configured with a [cert-manager](https://cert-manager.io/docs/) integration that automatically creates a self-signed issuer and TLS certificate. This can be disabled if you want to bring your own TLS certificate.

### With cert-manager

Deploy the Helm chart with default values. This enables the cert-manager integration and creates a self-signed issuer:

```sh
helm upgrade pod-image-policy ./helm/pod-image-policy \
  --install \
  --namespace pod-image-policy \
  --create-namespace
```

To use an existing cert-manager issuer instead of the self-signed one:

```sh
helm upgrade pod-image-policy ./helm/pod-image-policy \
  --install \
  --namespace pod-image-policy \
  --create-namespace \
  --set certManager.createSelfSignedIssuer=false \
  --set certManager.issuerRef.name=my-issuer \
  --set certManager.issuerRef.kind=ClusterIssuer
```

### Without cert-manager

If you prefer to manage certificates manually without cert-manager:

1. Create namespace and TLS secret:

   ```sh
   kubectl create namespace pod-image-policy

   openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
     -keyout /tmp/tls.key -out /tmp/tls.crt \
     -subj "/CN=pod-image-policy.pod-image-policy.svc" \
     -addext "subjectAltName=DNS:pod-image-policy.pod-image-policy.svc,DNS:pod-image-policy.pod-image-policy.svc.cluster.local"

   kubectl create secret tls pod-image-policy-tls \
     --cert=/tmp/tls.crt --key=/tmp/tls.key -n pod-image-policy
   ```

2. Deploy the Helm chart:

   ```sh
   helm upgrade pod-image-policy ./helm/pod-image-policy \
     --install \
     --namespace pod-image-policy \
     --set certManager.enabled=false \
     --set webhooks.mutating.caBundle=$(cat /tmp/tls.crt | base64 | tr -d '\n') \
     --set webhooks.validating.caBundle=$(cat /tmp/tls.crt | base64 | tr -d '\n') \
     --set-json 'volumes=[{"name":"tls","secret":{"secretName":"pod-image-policy-tls"}}]' \
     --set-json 'volumeMounts=[{"name":"tls","mountPath":"/tls","readOnly":true}]' \
     --set-json 'args=["-certFile=/tls/tls.crt","-keyFile=/tls/tls.key"]'
   ```

### Custom policy configuration

By default, the webhook allows all images without mutations or restrictions. To enforce custom image policies, configure the `policy` value with your desired validation and mutation rules.

**Using a values file (recommended):**

Create `custom-values.yaml`:

```yaml
policy:
  mutate:
    rules:
      - match:
          registry: "^docker\.io$"
          repository: "^library/(.*)$"
        replace:
          registry: "registry.internal"
          repository: "mirror/library/${1}"
        message: "Image rewritten to use internal mirror"
  validate:
    rules:
      - match:
          tag: "^latest$"
        action: deny
        message: "'latest' tags are not allowed"
```

Deploy with custom policy:

```sh
helm upgrade pod-image-policy ./helm/pod-image-policy \
  --install \
  --namespace pod-image-policy \
  --create-namespace \
  --values custom-values.yaml
```
