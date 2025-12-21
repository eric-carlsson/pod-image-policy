# pod-image-policy

Kubernetes admission webhook that rewrites pod image references and enforces validation policies.

## Features

- Mutate pod `image` fields via glob-matched rewrite rules.
- Validate pod `image` fields with allow/deny/warn actions and custom messages
- Expressive policy configuration with hot-reloading.

## Examples

Use the controller to validate or mutate image references; here are a few common patterns.

### Mutate: rewrite to internal mirror

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

### Validate: reject `:latest` tags

```yaml
config:
  validate:
    rules:
      - match:
          tag: "latest"
        action: deny
        message: "Pin image tags (no :latest)"
```
