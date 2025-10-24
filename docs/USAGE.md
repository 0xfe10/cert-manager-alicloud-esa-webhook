# Usage Guide

This document provides detailed instructions for using **cert-manager-alicloud-esa-webhook**.

## Prerequisites

1. **Kubernetes Cluster**: cert-manager v1.15+ is installed
2. **Alibaba Cloud Account**: ESA (Edge Security Acceleration) is enabled
3. **Domain Management**: The target domain is managed within Alibaba Cloud ESA

## Installation Steps

### 1) Create Alibaba Cloud Access Credentials

First, obtain your Alibaba Cloud AccessKey ID and AccessKey Secret:

1. Log in to the Alibaba Cloud Console
2. Click your avatar in the upper-right corner and choose **AccessKey Management**
3. Create a new AccessKey or use an existing one
4. Ensure the AccessKey has the required ESA permissions

### 2) Create a Kubernetes Secret

Store your Alibaba Cloud credentials as a Kubernetes Secret:

```bash
# Base64-encode your credentials
echo -n "your-access-key-id" | base64
echo -n "your-access-key-secret" | base64

# Create secret.yaml
cat <<EOF > secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: alicloud-esa-secret
  namespace: cert-manager
type: Opaque
data:
  access-key-id: <base64-encoded-access-key-id>
  access-key-secret: <base64-encoded-access-key-secret>
EOF

# Apply to the cluster
kubectl apply -f secret.yaml
```

### 3) Deploy the Webhook

Use Helm to deploy the webhook:

```bash
# Add the chart repo (if applicable)
helm repo add alicloud-esa-webhook https://your-repo.com/charts
helm repo update

# Or deploy from a local path
helm install alicloud-esa-webhook ./deploy/alicloud-esa-webhook \
  --namespace cert-manager \
  --set groupName=acme.esa.alicloud.com
```

### 4) Create an Issuer

Create a cert-manager Issuer that uses this webhook:

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: letsencrypt-staging
  namespace: default
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-staging-account-key
    solvers:
    - dns01:
        webhook:
          groupName: acme.esa.alicloud.com
          solverName: alicloud-esa-solver
          config:
            regionId: "ap-southeast-1"  # Set to the region of your ESA instance
            accessKeyIdSecretRef:
              name: alicloud-esa-secret
              key: access-key-id
            accessKeySecretSecretRef:
              name: alicloud-esa-secret
              key: access-key-secret
```

### 5) Request a Certificate

Create a `Certificate` resource to request a certificate:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-com-tls
  namespace: default
spec:
  secretName: example-com-tls
  issuerRef:
    name: letsencrypt-staging
    kind: Issuer
  dnsNames:
  - example.com
  - "*.example.com"
```

## Configuration Parameters

| Parameter                       | Description                                  | Required | Example                 |
| ------------------------------- | -------------------------------------------- | -------- | ----------------------- |
| `regionId`                      | Alibaba Cloud ESA region ID                  | Yes      | `"ap-southeast-1"`      |
| `accessKeyIdSecretRef.name`     | Secret name that stores the AccessKey ID     | Yes      | `"alicloud-esa-secret"` |
| `accessKeyIdSecretRef.key`      | Key in the Secret for AccessKey ID           | Yes      | `"access-key-id"`       |
| `accessKeySecretSecretRef.name` | Secret name that stores the AccessKey Secret | Yes      | `"alicloud-esa-secret"` |
| `accessKeySecretSecretRef.key`  | Key in the Secret for AccessKey Secret       | Yes      | `"access-key-secret"`   |

## Supported Regions

The webhook supports all Alibaba Cloud regions where ESA is available, including but not limited to:

- `ap-southeast-1`
- `cn-hangzhou`

## Troubleshooting

### Common Issues

1. **Permission Errors**

   * Ensure the AccessKey has the necessary ESA permissions
   * Verify ESA is enabled in the specified region

2. **Domain Not Found**

   * Confirm the domain is configured in Alibaba Cloud ESA
   * Check that the domain status is normal

3. **Network Connectivity Problems**

   * Ensure the Kubernetes cluster can reach Alibaba Cloud APIs
   * Review firewall rules and network policies

### Debugging Steps

1. Check webhook logs:

```bash
kubectl logs -n cert-manager deployment/alicloud-esa-webhook
```

2. Check cert-manager logs:

```bash
kubectl logs -n cert-manager deployment/cert-manager
```

3. Inspect Challenge status:

```bash
kubectl describe challenge
```

4. Inspect Order status:

```bash
kubectl describe order
```

## Production Considerations

1. **Use the Production ACME Server**:

   ```yaml
   server: https://acme-v02.api.letsencrypt.org/directory  # Production
   ```

2. **Set Resource Limits**:

   ```yaml
   resources:
     limits:
       cpu: 100m
       memory: 128Mi
     requests:
       cpu: 50m
       memory: 64Mi
   ```

3. **Run Multiple Replicas**:

   ```yaml
   replicaCount: 2
   ```

4. **Monitoring & Alerts**:

   * Monitor certificate expirations
   * Configure health check alerts for the webhook

## Security Recommendations

1. **Principle of Least Privilege**: Grant only the minimal permissions required for ESA DNS record management
2. **Credential Rotation**: Rotate AccessKeys regularly
3. **Network Isolation**: Use NetworkPolicies to restrict webhook egress/ingress
4. **Audit Logging**: Enable Kubernetes audit logs to track certificate operations
