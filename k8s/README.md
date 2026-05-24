# MozartPay Kubernetes Deployment Files

This directory contains all the Kubernetes manifests for deploying MozartPay services.

## File Structure

```
k8s/
├── 00-namespace.yaml      # Namespace, ConfigMaps, Secrets, RBAC
├── 01-postgres.yaml        # PostgreSQL database deployment
├── 02-redis.yaml           # Redis cache deployment
├── 03-webauthn.yaml        # WebAuthn service with autoscaling
├── 04-mcp.yaml             # MCP server (SSE + stdio Job)
├── 05-horizon.yaml         # Stellar Horizon instances
├── 06-networking.yaml      # Ingress, NetworkPolicy, Services
└── 07-monitoring.yaml      # Prometheus + Grafana monitoring
```

## Deployment Order

The manifests are numbered to ensure proper deployment order:

1. **Infrastructure** (00-02): Namespace, database, cache
2. **Application Services** (03-05): WebAuthn, MCP, Horizon
3. **Networking** (06): Ingress and network policies
4. **Monitoring** (07): Prometheus and Grafana

## Quick Reference

### Deploy All Services
```bash
kubectl apply -f k8s/
```

### Deploy Individual Components
```bash
# Infrastructure
kubectl apply -f k8s/00-namespace.yaml
kubectl apply -f k8s/01-postgres.yaml
kubectl apply -f k8s/02-redis.yaml

# Services
kubectl apply -f k8s/03-webauthn.yaml
kubectl apply -f k8s/04-mcp.yaml
kubectl apply -f k8s/05-horizon.yaml

# Networking
kubectl apply -f k8s/06-networking.yaml

# Monitoring
kubectl apply -f k8s/07-monitoring.yaml
```

### Check Deployment Status
```bash
kubectl get all -n mozartpay
kubectl get pods -n mozartpay -w
```

### Access Services
```bash
# Port forwarding
kubectl port-forward service/webauthn 8000:8000 -n mozartpay
kubectl port-forward service/mcp-sse 3000:3000 -n mozartpay
kubectl port-forward service/horizon-testnet 8001:8000 -n mozartpay

# Monitoring
kubectl port-forward service/prometheus 9090:9090 -n mozartpay
kubectl port-forward service/grafana 3001:3000 -n mozartpay
```

## Configuration

### Environment Variables

Key environment variables that can be customized:

- **NETWORK**: stellar-testnet or stellar-mainnet
- **LOG_LEVEL**: debug, info, warn, error
- **TRANSPORT**: stdio or sse (MCP only)
- **HORIZON_URL**: Custom Horizon endpoint

### Secrets

Update the following in `k8s/00-namespace.yaml`:

```yaml
data:
  postgres-password: <base64-encoded-password>
  postgres-user: <base64-encoded-username>
  bitpanda-api-key: <base64-encoded-api-key>
  bitpanda-api-secret: <base64-encoded-api-secret>
```

### Resource Limits

Adjust resource requests and limits in each deployment manifest based on your cluster capacity.

## Scaling

### Horizontal Pod Autoscaling

All services include HPA configuration:

- **WebAuthn**: 2-10 replicas
- **MCP**: 2-8 replicas  
- **Horizon**: 1-5 replicas

### Manual Scaling

```bash
kubectl scale deployment webauthn --replicas=5 -n mozartpay
kubectl scale deployment mcp-sse --replicas=4 -n mozartpay
kubectl scale deployment horizon-testnet --replicas=3 -n mozartpay
```

## Monitoring

### Prometheus Metrics

Access Prometheus metrics at:
- WebAuthn: http://localhost:8000/metrics
- MCP: http://localhost:3000/metrics
- Horizon: http://localhost:8001/metrics

### Grafana Dashboards

- **URL**: http://localhost:3001
- **Username**: admin
- **Password**: mozartpay123

### Alerting Rules

Pre-configured alerts for:
- Service downtime
- High resource usage
- Database connectivity issues

## Security

### Network Policies

Default deny policy with specific allow rules:
- Ingress controller access
- Same namespace communication
- DNS and HTTP/HTTPS egress

### Pod Security

- Non-root users
- Read-only filesystems (where applicable)
- Dropped capabilities
- Resource constraints

### Secrets Management

All sensitive data stored in Kubernetes secrets with proper access controls.

## Troubleshooting

### Common Issues

1. **Pods stuck in Pending**
   ```bash
   kubectl describe pod <pod-name> -n mozartpay
   kubectl get pvc -n mozartpay
   ```

2. **Service not accessible**
   ```bash
   kubectl get endpoints -n mozartpay
   kubectl get svc -n mozartpay
   ```

3. **High resource usage**
   ```bash
   kubectl top pods -n mozartpay
   kubectl get hpa -n mozartpay
   ```

### Debug Commands

```bash
# View logs
kubectl logs -f deployment/webauthn -n mozartpay

# Exec into container
kubectl exec -it deployment/webauthn -n mozartpay -- sh

# Check events
kubectl get events -n mozartpay --sort-by=.lastTimestamp

# Describe resources
kubectl describe deployment/webauthn -n mozartpay
```

## Customization

### Adding New Services

1. Create new deployment manifest with proper labels
2. Add service and HPA configuration
3. Update network policies if needed
4. Add monitoring configuration

### Environment-Specific Configurations

Create separate ConfigMaps for different environments:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mozartpay-config-prod
  namespace: mozartpay
data:
  config.json: |
    {
      "network": "stellar-mainnet",
      "debug": false
    }
```

### Custom Ingress

Modify `k8s/06-networking.yaml` for:
- Different domains
- SSL certificates
- Advanced routing rules
- Rate limiting configurations

## Backup and Recovery

### Database Backups

```bash
# Create backup
kubectl exec -it postgres -n mozartpay -- pg_dump -U mozartpay mozartpay > backup.sql

# Restore backup
kubectl exec -i postgres -n mozartpay -- psql -U mozartpay mozartpay < backup.sql
```

### Volume Snapshots

Use your cloud provider's snapshot tools for persistent volume backups.

## Production Considerations

### High Availability

- Deploy across multiple availability zones
- Use anti-affinity rules
- Configure proper resource limits
- Set up backup and disaster recovery

### Performance Optimization

- Tune resource requests/limits
- Configure appropriate HPA thresholds
- Use proper storage classes
- Monitor and optimize database queries

### Cost Management

- Right-size resources
- Use spot instances where appropriate
- Implement auto-scaling
- Monitor resource utilization
