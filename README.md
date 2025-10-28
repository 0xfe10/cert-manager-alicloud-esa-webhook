# Alibaba Cloud ESA ACME webhook

This is a webhook solver for [cert-manager](https://cert-manager.io/) that can be used to issue ACME certificates by using Alibaba Cloud Edge Security Acceleration (ESA) DNS01 challenges.

## Features

- Supports DNS01 challenges using Alibaba Cloud ESA
- Compatible with cert-manager v1.15+
- Secure credential management through Kubernetes secrets

## Installation

### Prerequisites

- Kubernetes cluster with cert-manager installed
- Alibaba Cloud account with ESA service enabled
- A domain managed by Alibaba Cloud ESA

### Deploy the webhook

1. Create a secret with your Alibaba Cloud credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: alicloud-esa-secret
  namespace: cert-manager
data:
  access-key-id: <base64-encoded-access-key-id>
  access-key-secret: <base64-encoded-access-key-secret>
```

2. Deploy the webhook:

```bash
kubectl apply -f deploy/
```

3. Create an Issuer using the webhook:

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-staging
    solvers:
    - dns01:
        webhook:
          groupName: acme.yourcompany.com
          solverName: alicloud-esa-solver
          config:
            regionId: "ap-southeast-1"
            accessKeyIdSecretRef:
              name: alicloud-esa-secret
              key: access-key-id
            accessKeySecretSecretRef:
              name: alicloud-esa-secret
              key: access-key-secret
```

## Configuration

The webhook accepts the following configuration parameters:

| Parameter | Description | Required |
|-----------|-------------|----------|
| `regionId` | Alibaba Cloud region ID (e.g., "ap-southeast-1") | Yes |
| `accessKeyIdSecretRef` | Reference to Kubernetes secret containing AccessKey ID | Yes |
| `accessKeySecretSecretRef` | Reference to Kubernetes secret containing AccessKey Secret | Yes |

## Supported Regions

The webhook supports all Alibaba Cloud regions where ESA service is available. The most commonly used regions are:

- `ap-southeast-1` (Singapore)
- `cn-hangzhou` (Hangzhou)

## Development

### Building

```bash
go build -o webhook .
```

### Testing

```bash
go test -v .
```

### Creating a new release

```bash
docker build -t your-registry/cert-manager-alicloud-esa-webhook:latest .
docker push your-registry/cert-manager-alicloud-esa-webhook:latest
```

## License

This project is licensed under the Apache License 2.0 - see the LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for your changes
5. Submit a pull request

## Support

For issues and feature requests, please create an issue in the GitHub repository.
