# 使用示例

本文档提供了 cert-manager-alicloud-esa-webhook 的详细使用说明。

## 前置条件

1. **Kubernetes 集群**：确保已安装 cert-manager v1.15+
2. **阿里云账号**：开通了 ESA (Edge Security Acceleration) 服务
3. **域名管理**：目标域名已在阿里云 ESA 中管理

## 安装步骤

### 1. 创建阿里云访问凭证

首先，需要获取阿里云的 AccessKey ID 和 AccessKey Secret：

1. 登录阿里云控制台
2. 在右上角头像处点击 "AccessKey管理"
3. 创建新的 AccessKey 或使用现有的
4. 确保该 AccessKey 具有 ESA 相关权限

### 2. 创建 Kubernetes Secret

将阿里云凭证存储为 Kubernetes Secret：

```bash
# 对凭证进行 base64 编码
echo -n "your-access-key-id" | base64
echo -n "your-access-key-secret" | base64

# 创建 secret.yaml
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

# 应用到集群
kubectl apply -f secret.yaml
```

### 3. 部署 webhook

使用 Helm 部署 webhook：

```bash
# 添加 chart 仓库（如果有的话）
helm repo add alicloud-esa-webhook https://your-repo.com/charts
helm repo update

# 或者直接从本地部署
helm install alicloud-esa-webhook ./deploy/alicloud-esa-webhook \
  --namespace cert-manager \
  --set groupName=acme.esa.alicloud.com
```

### 4. 创建 Issuer

创建一个使用该 webhook 的 cert-manager Issuer：

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
            regionId: "ap-southeast-1"  # 根据你的 ESA 实例区域设置
            accessKeyIdSecretRef:
              name: alicloud-esa-secret
              key: access-key-id
            accessKeySecretSecretRef:
              name: alicloud-esa-secret
              key: access-key-secret
```

### 5. 申请证书

创建 Certificate 资源来申请证书：

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

## 配置参数说明

| 参数 | 描述 | 必需 | 示例值 |
|------|------|------|--------|
| `regionId` | 阿里云 ESA 服务区域ID | 是 | "ap-southeast-1" |
| `accessKeyIdSecretRef.name` | 包含 AccessKey ID 的 Secret 名称 | 是 | "alicloud-esa-secret" |
| `accessKeyIdSecretRef.key` | Secret 中 AccessKey ID 的键名 | 是 | "access-key-id" |
| `accessKeySecretSecretRef.name` | 包含 AccessKey Secret 的 Secret 名称 | 是 | "alicloud-esa-secret" |
| `accessKeySecretSecretRef.key` | Secret 中 AccessKey Secret 的键名 | 是 | "access-key-secret" |

## 支持的区域

webhook 支持所有提供 ESA 服务的阿里云区域：

- `ap-southeast-1`
- `cn-hangzhou`

## 故障排除

### 常见问题

1. **权限错误**
   - 确保 AccessKey 具有 ESA 相关权限
   - 检查 ESA 服务是否已在指定区域开通

2. **域名不存在**
   - 确保域名已在阿里云 ESA 中配置
   - 检查域名状态是否正常

3. **网络连接问题**
   - 确保 Kubernetes 集群可以访问阿里云 API
   - 检查防火墙和网络策略设置

### 调试步骤

1. 查看 webhook 日志：
```bash
kubectl logs -n cert-manager deployment/alicloud-esa-webhook
```

2. 查看 cert-manager 日志：
```bash
kubectl logs -n cert-manager deployment/cert-manager
```

3. 检查 Challenge 状态：
```bash
kubectl describe challenge
```

4. 检查 Order 状态：
```bash
kubectl describe order
```

## 生产环境注意事项

1. **使用生产环境 ACME 服务器**：
   ```yaml
   server: https://acme-v02.api.letsencrypt.org/directory  # 生产环境
   ```

2. **设置资源限制**：
   ```yaml
   resources:
     limits:
       cpu: 100m
       memory: 128Mi
     requests:
       cpu: 50m
       memory: 64Mi
   ```

3. **配置多副本**：
   ```yaml
   replicaCount: 2
   ```

4. **监控和告警**：
   - 设置证书到期监控
   - 配置 webhook 健康检查告警

## 安全建议

1. **最小权限原则**：只授予 ESA DNS 记录管理的最小权限
2. **定期轮换凭证**：定期更新 AccessKey
3. **网络隔离**：使用网络策略限制 webhook 的网络访问
4. **审计日志**：启用 Kubernetes 审计日志来跟踪证书操作
