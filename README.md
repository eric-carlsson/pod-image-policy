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

1. Create namespace and TLS secret (example self-signed):

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
     --set image.repository=your-registry/pod-image-policy \
     --set image.tag=latest \
     --set webhook.mutating.caBundle=$(cat /tmp/tls.crt | base64 | tr -d '\n') \
     --set-json 'volumes=[{"name":"certs","secret":{"secretName":"pod-image-policy-tls"}}]' \
     --set-json 'volumeMounts=[{"name":"certs","mountPath":"/certs","readOnly":true}]' \
     --set-json 'args=["-certFile=/certs/tls.crt","-keyFile=/certs/tls.key","-policyFile=/config/policy.yaml","-addr=:9443"]' \
     --set-json 'config={"mutate":{"rules":[{"match":{"registry":"^docker\\.io$","repository":"^library/(.*)$"},"replace":{"registry":"registry.internal","repository":"mirror/library/${1}"}}]}}'
   ```

See Taskfile targets for local/dev deployment options.
