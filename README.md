# pod-image-policy

Kubernetes admission controller for rewriting and enforcing policies on pod `image` fields.

## Features

- Mutate pod `image` fields via glob-matched rewrite rules.
- Validate pod `image` fields with allow/deny/warn actions and custom messages.

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

2. Deploy Helm chart.

   ```sh
   helm upgrade pod-image-policy ./helm/pod-image-policy \
     --install \
     --namespace pod-image-policy \
     --set image.repository=your-registry/pod-image-policy \
     --set image.tag=latest \
     --set webhook.mutating.caBundle=$(cat /tmp/tls.crt | base64 | tr -d '\n') \
     --set-json 'volumes=[{"name":"certs","secret":{"secretName":"pod-image-policy-tls"}}]' \
     --set-json 'volumeMounts=[{"name":"certs","mountPath":"/certs","readOnly":true}]' \
     --set-json 'args=["-certFile=/certs/tls.crt","-keyFile=/certs/tls.key","-configFile=/config/policy.yaml","-addr=:9443"]' \
     --set-json 'config={"mutate":{"rules":[{"match":{"registry":"docker.io","repository":"library/*"},"replace":{"registry":"registry.internal","repository":"mirror/{$1}"}}]}}'
   ```

See Taskfile targets for a local/dev deployment options.

## Examples

Use the controller to validate or mutate image references; here are a few common patterns.

- Rewrite all images matching `docker.io/library/<image>` (or `<image>`) to `registry.internal/mirror/library/<image>`.

  ```yaml
  config:
    mutate:
      rules:
        - match:
            registry: "docker.io"
            repository: "library/*"
          replace:
            registry: "registry.internal"
            repository: "mirror/{$1}"
  ```

- Reject all images with `latest` tag.

  ```yaml
  config:
    validate:
      rules:
        - match:
            tag: "latest"
          action: deny
          message: "Pin image tags (no :latest)"
  ```
