# KVStore Configuration Management

This directory contains the configuration management setup for the distributed KVStore application, supporting multiple deployment methods and environments.

## Directory Structure

```
config-management/
├── helm/                    # Helm charts
│   └── kvstore/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── kustomize/              # Kustomize configurations
│   ├── base/               # Base configuration
│   ├── overlays/           # Environment-specific overlays
│   │   ├── dev/
│   │   ├── staging/
│   │   └── production/
│   └── transformers/       # Configuration transformers
├── argocd/                 # ArgoCD applications
│   ├── applications/
│   └── projects/
└── README.md
```

## Deployment Methods

### 1. Helm Deployment

#### Install from Local Chart
```bash
# Development environment
helm install kvstore-dev ./helm/kvstore \
  --namespace kvstore-dev \
  --create-namespace \
  --values helm/kvstore/values-dev.yaml

# Production environment
helm install kvstore-prod ./helm/kvstore \
  --namespace kvstore \
  --create-namespace \
  --values helm/kvstore/values-prod.yaml
```

#### Install from Registry
```bash
# Add helm repository
helm repo add kvstore https://charts.example.com/kvstore
helm repo update

# Install
helm install kvstore kvstore/kvstore \
  --version 1.0.0 \
  --namespace kvstore \
  --create-namespace
```

### 2. Kustomize Deployment

#### Development Environment
```bash
kubectl apply -k kustomize/overlays/dev
```

#### Production Environment
```bash
kubectl apply -k kustomize/overlays/production
```

#### Dry Run and Validation
```bash
# Preview changes
kubectl diff -k kustomize/overlays/production

# Validate configuration
kubectl apply -k kustomize/overlays/production --dry-run=client
```

### 3. ArgoCD GitOps

#### Deploy ArgoCD Applications
```bash
# Apply application definitions
kubectl apply -f argocd/applications/

# Monitor deployment
argocd app list
argocd app get kvstore-production
argocd app sync kvstore-production
```

## Configuration Customization

### Environment Variables

| Variable | Description | Default | Environments |
|----------|-------------|---------|--------------|
| `CLUSTER_SIZE` | Number of nodes in cluster | `3` | dev: 3, prod: 5 |
| `REPLICATION_FACTOR` | Data replication factor | `2` | dev: 2, prod: 3 |
| `LOG_LEVEL` | Application log level | `INFO` | dev: DEBUG, prod: INFO |
| `STORAGE_SIZE` | Persistent volume size | `20Gi` | dev: 10Gi, prod: 100Gi |
| `MONITORING_ENABLED` | Enable monitoring | `true` | All |
| `TRACING_ENABLED` | Enable distributed tracing | `true` | All |

### Resource Configuration

#### Development Resources
```yaml
resources:
  requests:
    cpu: 100m
    memory: 256Mi
  limits:
    cpu: 500m
    memory: 1Gi
```

#### Production Resources
```yaml
resources:
  requests:
    cpu: 500m
    memory: 2Gi
  limits:
    cpu: 2000m
    memory: 8Gi
```

### Storage Configuration

#### Storage Classes
- **Development**: `gp3` (general purpose SSD)
- **Production**: `gp3` with higher IOPS and throughput

#### Backup Configuration
```yaml
backup:
  enabled: true
  schedule: "0 2 * * *"  # Daily at 2 AM
  retention:
    days: 30             # dev: 7, prod: 90
  s3:
    bucket: "kvstore-backups-${environment}"
    encryption: true
```

## Security Configuration

### TLS/SSL
```yaml
security:
  tls:
    enabled: true        # prod: true, dev: false
    secretName: kvstore-tls
```

### Network Policies
```yaml
networkPolicy:
  enabled: true
  ingress:
    - from:
      - namespaceSelector:
          matchLabels:
            name: monitoring
    - from:
      - namespaceSelector:
          matchLabels:
            name: ingress-nginx
```

### Pod Security Standards
```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000
  capabilities:
    drop:
      - ALL
  readOnlyRootFilesystem: true
```

## Monitoring and Observability

### Prometheus Integration
```yaml
monitoring:
  prometheus:
    enabled: true
    serviceMonitor:
      enabled: true
      interval: 30s
    prometheusRule:
      enabled: true
```

### Grafana Dashboards
- **KVStore Overview**: Cluster health and performance
- **Node Metrics**: Individual node performance
- **Raft Consensus**: Consensus algorithm metrics
- **Storage Metrics**: Disk usage and performance

### Alerting Rules
- High memory usage (>80%)
- High CPU usage (>70%)
- Raft leader election failures
- Storage volume usage (>85%)
- Pod restart loops
- Service availability

## Disaster Recovery

### Backup Strategy
- **Automated Backups**: Daily incremental, weekly full
- **Cross-Region Replication**: Multi-region backup storage
- **Recovery Testing**: Weekly automated recovery tests

### Recovery Procedures
```bash
# List available backups
kubectl exec -n kvstore kvstore-0 -- backup list

# Restore from backup
kubectl exec -n kvstore kvstore-0 -- backup restore \
  --backup-id "20231201-020000" \
  --target-cluster kvstore-recovery
```

## Environment Promotion

### Development to Staging
```bash
# Update staging overlay with new image tag
cd kustomize/overlays/staging
kustomize edit set image kvstore=ghcr.io/distributed-kvstore/kvstore:v1.2.0

# Apply changes
kubectl apply -k .
```

### Staging to Production
```bash
# Create pull request with production updates
git checkout -b promote-v1.2.0-to-prod
cd kustomize/overlays/production
kustomize edit set image kvstore=ghcr.io/distributed-kvstore/kvstore:v1.2.0
git commit -am "Promote v1.2.0 to production"
git push origin promote-v1.2.0-to-prod
```

## Troubleshooting

### Common Issues

#### Pod Startup Issues
```bash
# Check pod status
kubectl get pods -n kvstore

# Check pod logs
kubectl logs -n kvstore kvstore-0 --previous

# Describe pod for events
kubectl describe pod -n kvstore kvstore-0
```

#### Storage Issues
```bash
# Check PVC status
kubectl get pvc -n kvstore

# Check storage class
kubectl get storageclass

# Check node disk usage
kubectl top nodes
```

#### Networking Issues
```bash
# Check service endpoints
kubectl get endpoints -n kvstore

# Test internal connectivity
kubectl exec -n kvstore kvstore-0 -- nc -zv kvstore-1.kvstore-headless 8080
```

### Performance Tuning

#### JVM Tuning
```yaml
env:
  - name: JAVA_OPTS
    value: "-Xms2g -Xmx6g -XX:+UseG1GC -XX:MaxGCPauseMillis=200"
```

#### Storage Tuning
```yaml
persistence:
  storageClass: gp3
  annotations:
    volume.beta.kubernetes.io/storage-provisioner: ebs.csi.aws.com
    volume.beta.kubernetes.io/storage-class: gp3
```

## Best Practices

1. **Configuration Management**
   - Use environment-specific overlays
   - Maintain configuration in version control
   - Use secrets for sensitive data

2. **Deployment Strategy**
   - Blue-green deployments for production
   - Canary deployments for gradual rollouts
   - Automated rollback on failure

3. **Monitoring**
   - Comprehensive health checks
   - Application-specific metrics
   - Alerting on critical conditions

4. **Security**
   - Regular security scanning
   - Principle of least privilege
   - Network segmentation

5. **Backup and Recovery**
   - Regular backup testing
   - Cross-region backup storage
   - Documented recovery procedures

## Support

For questions or issues:
- **Documentation**: https://docs.example.com/kvstore
- **Runbook**: https://runbook.example.com/kvstore
- **Slack**: #kvstore-support
- **Email**: platform@example.com