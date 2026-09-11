# MozartPay Kubernetes Deployment

This directory contains the complete Kubernetes deployment configuration for MozartPay services.

## Architecture Overview

The deployment includes the following services:

- **WebAuthn Server** - Passkey authentication service (port 8000)
- **MCP Server** - Model Context Protocol for AI integration (port 3000)
- **Horizon Instances** - Stellar network connectivity (port 8000)
- **PostgreSQL** - Primary database for persistent storage
- **Redis** - Caching and session storage
- **Prometheus** - Metrics collection and monitoring
- **Grafana** - Visualization and dashboards

## Quick Start

### Prerequisites

- Kubernetes cluster (minikube, kind, k3s, or cloud provider)
- `kubectl` configured to access your cluster
- `docker` for building images
- Sufficient cluster resources (minimum 4GB RAM, 2 CPU cores)

### Deployment

1. **Build and deploy all services:**
   ```bash
   ./scripts/deploy.sh
   ```

2. **Monitor deployment progress:**
   ```bash
   kubectl get pods -n mozartpay
   kubectl get services -n mozartpay
   ```

3. **Access services:**
   ```bash
   # Port forwarding for local access
   kubectl port-forward service/webauthn 8000:8000 -n mozartpay
   kubectl port-forward service/mcp-sse 3000:3000 -n mozartpay
   kubectl port-forward service/horizon-testnet 8001:8000 -n mozartpay
   ```

### Cleanup

To remove all deployed resources:
```bash
./scripts/cleanup.sh
```

## Service Details

### WebAuthn Service

- **Image**: `mozartpay/webauthn:latest`
- **Replicas**: 2 (auto-scalable to 10)
- **Port**: 8000
- **Health Check**: `/health` endpoint
- **Features**: 
  - Horizontal Pod Autoscaling
  - Liveness and readiness probes
  - Security context with non-root user

### MCP Server

- **Image**: `mozartpay/mcp:latest`
- **Replicas**: 3 (auto-scalable to 8)
- **Port**: 3000
- **Transport Modes**: SSE (default), stdio (Job)
- **Features**:
  - Auto-scaling based on CPU/memory usage
  - Health monitoring
  - Configurable logging levels

### Horizon Instances

- **Image**: `stellar/stellar-horizon:latest`
- **Replicas**: 2 (auto-scalable to 5)
- **Port**: 8000
- **Database**: PostgreSQL backend
- **Features**:
  - Testnet configuration
  - Persistent storage
  - Database initialization

### Infrastructure Services

#### PostgreSQL
- **Version**: 15-alpine
- **Storage**: 10Gi persistent volume
- **Databases**: mozartpay, horizon_testnet, stellar_core_testnet
- **Security**: Encrypted password storage

#### Redis
- **Version**: 7-alpine
- **Storage**: 2Gi persistent volume
- **Memory Limit**: 256MB
- **Eviction Policy**: allkeys-lru

## Networking

### Ingress Configuration

The deployment includes an NGINX ingress controller with:

- **SSL Termination**: Automatic HTTPS with Let's Encrypt
- **Rate Limiting**: 100 requests per minute
- **Path-based Routing**: 
  - `/webauthn/*` → WebAuthn service
  - `/mcp/*` → MCP service
  - `/horizon-testnet/*` → Horizon service

### Network Policies

- **Default Deny**: All traffic blocked by default
- **Allowed Sources**: Ingress controller, same namespace pods
- **Egress Rules**: DNS, HTTP/HTTPS outbound, internal service communication

### Service Types

- **ClusterIP**: Internal service communication
- **LoadBalancer**: External access (if supported by cluster)
- **NodePort**: Alternative external access method

## Monitoring and Observability

### Prometheus

- **Scrape Interval**: 15 seconds
- **Metrics Endpoints**: All services expose `/metrics`
- **Alerting Rules**: Service downtime, high resource usage
- **Storage**: In-cluster (emptyDir)

### Grafana

- **Admin Password**: `mozartpay123`
- **Plugins**: Kubernetes app
- **Data Sources**: Prometheus
- **Dashboards**: Pre-configured for MozartPay services

### Alerting Rules

- **Critical**: WebAuthn/MCP service down
- **Warning**: Horizon service down, high resource usage
- **Thresholds**: 90% memory, 80% CPU usage

## Security

### Container Security

- **Non-root Users**: All containers run as non-root
- **Read-only Filesystem**: Where applicable
- **Capability Dropping**: All capabilities dropped
- **Resource Limits**: Memory and CPU constraints

### Secrets Management

- **Kubernetes Secrets**: Encrypted at rest
- **Base64 Encoding**: All secret values
- **Environment Injection**: Secure credential passing

### RBAC

- **Service Accounts**: Dedicated accounts per component
- **Least Privilege**: Minimal required permissions
- **Role Bindings**: Namespace-scoped access

## Scaling and Performance

### Horizontal Pod Autoscaling

- **WebAuthn**: 2-10 replicas (70% CPU, 80% memory)
- **MCP**: 2-8 replicas (70% CPU, 80% memory)
- **Horizon**: 1-5 replicas (70% CPU, 80% memory)

### Resource Requests and Limits

| Service | CPU Request | CPU Limit | Memory Request | Memory Limit |
|---------|-------------|-----------|---------------|--------------|
| WebAuthn | 100m | 200m | 128Mi | 256Mi |
| MCP | 200m | 400m | 256Mi | 512Mi |
| Horizon | 300m | 600m | 512Mi | 1Gi |
| PostgreSQL | 250m | 500m | 256Mi | 512Mi |
| Redis | 100m | 200m | 128Mi | 256Mi |

## Configuration

### Environment Variables

Key configuration options:

- **NETWORK**: stellar-testnet or stellar-mainnet
- **LOG_LEVEL**: debug, info, warn, error
- **TRANSPORT**: stdio or sse (MCP only)
- **HORIZON_URL**: Custom Horizon endpoint

### ConfigMaps

- **mozartpay-config**: Application configuration
- **prometheus-config**: Monitoring configuration
- **logging.yaml**: Logging configuration

### Secrets

- **postgres-password**: Database password
- **postgres-user**: Database username
- **bitpanda-api-key**: Exchange API key (optional)
- **bitpanda-api-secret**: Exchange API secret (optional)

## Troubleshooting

### Common Issues

1. **Pods stuck in Pending state**
   - Check cluster resources: `kubectl describe nodes`
   - Verify PVCs: `kubectl get pvc -n mozartpay`

2. **Service not accessible**
   - Check service endpoints: `kubectl get endpoints -n mozartpay`
   - Verify pod status: `kubectl get pods -n mozartpay`

3. **High resource usage**
   - Check HPA status: `kubectl get hpa -n mozartpay`
   - Monitor metrics: `kubectl top pods -n mozartpay`

### Debug Commands

```bash
# View pod logs
kubectl logs -f deployment/webauthn -n mozartpay

# Exec into pod
kubectl exec -it deployment/webauthn -n mozartpay -- sh

# Check events
kubectl get events -n mozartpay --sort-by='.lastTimestamp'

# Describe resources
kubectl describe deployment/webauthn -n mozartpay
```

## Development

### Local Development with Docker Compose

For local development without Kubernetes:

```bash
docker-compose up -d
```

This starts all services with local networking and volumes.

### Building Images

```bash
# Build all images
docker build -f Dockerfile -t mozartpay/cli:latest --target cli .
docker build -f Dockerfile.webauthn -t mozartpay/webauthn:latest --target webauthn .
docker build -f Dockerfile.mcp -t mozartpay/mcp:latest --target mcp .
```

### Testing

```bash
# Run tests locally
go test ./...

# Test in cluster
kubectl run test-pod --image=mozartpay/cli:latest -n mozartpay --rm -it -- ./mozartpay-cli --help
```

## Production Considerations

### Backup Strategy

- **Database**: Regular PostgreSQL backups
- **Configuration**: GitOps with version control
- **State**: Persistent volume snapshots

### Disaster Recovery

- **Multi-zone deployment**: Spread across availability zones
- **Cross-region replication**: For critical services
- **Backup restoration**: Tested recovery procedures

### Cost Optimization

- **Resource right-sizing**: Monitor actual usage
- **Spot instances**: For non-critical workloads
- **Autoscaling**: Scale down during low usage

## Support

For issues and questions:

1. Check the troubleshooting section
2. Review pod logs and events
3. Consult the Kubernetes documentation
4. Create an issue in the repository

## License

This deployment configuration is part of the MozartPay project and follows the same license terms.
