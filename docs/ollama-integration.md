# Ollama Integration with MozartPay

This document describes how to use the Ollama deployment within your MozartPay Kubernetes cluster.

## Overview

Ollama has been integrated into your existing MozartPay Kubernetes cluster to provide local AI model hosting capabilities. The deployment includes:

- **Persistent Storage**: 20GB PVC for model storage
- **Auto-scaling**: HPA configured for 1-3 replicas
- **Network Access**: Available via internal service, external load balancer, and ingress
- **Pre-loaded Models**: Your existing models (llama3.2:3b, qwen2.5:3b, deepseek-r1:1.5b) are automatically pulled

## Deployment Files

### Core Configuration
- `k8s/08-ollama.yaml` - Main Ollama deployment, service, and HPA
- `k8s/06-networking.yaml` - Updated with Ollama network policies and ingress rules

### Scripts
- `scripts/deploy-ollama.sh` - Automated deployment script

## Access Methods

### Internal Cluster Access
```
http://ollama:11434
```

### External Load Balancer Access
```
http://<loadbalancer-ip>:11434
```

### Ingress Access
```
https://mozartpay.example.com/ollama
https://api.mozartpay.example.com/ollama
```

## API Usage

### List Models
```bash
curl http://ollama:11434/api/tags
```

### Generate Completion
```bash
curl http://ollama:11434/api/generate -d '{
  "model": "llama3.2:3b",
  "prompt": "Why is the sky blue?",
  "stream": false
}'
```

### Chat Completion
```bash
curl http://ollama:11434/api/chat -d '{
  "model": "qwen2.5:3b",
  "messages": [
    {
      "role": "user",
      "content": "Hello, how are you?"
    }
  ],
  "stream": false
}'
```

## Management Commands

### Check Deployment Status
```bash
kubectl get pods -n mozartpay -l app.kubernetes.io/component=ollama
kubectl get service ollama -n mozartpay
kubectl get hpa ollama-hpa -n mozartpay
```

### View Logs
```bash
kubectl logs -n mozartpay deployment/ollama -f
```

### Manage Models
```bash
# List models
kubectl exec -n mozartpay deployment/ollama -- ollama list

# Pull new model
kubectl exec -n mozartpay deployment/ollama -- ollama pull llama3.1:8b

# Remove model
kubectl exec -n mozartpay deployment/ollama -- ollama remove llama3.2:3b

# Show model info
kubectl exec -n mozartpay deployment/ollama -- ollama show llama3.2:3b
```

### Scale Deployment
```bash
# Scale up
kubectl scale deployment ollama -n mozartpay --replicas=3

# Scale down
kubectl scale deployment ollama -n mozartpay --replicas=1
```

## Integration with MozartPay Services

Your MCP (Model Context Protocol) service can now connect to the Ollama service internally:

```go
// Example Go client for Ollama
client := &http.Client{
    Timeout: 30 * time.Second,
}

resp, err := client.Post(
    "http://ollama:11434/api/generate",
    "application/json",
    strings.NewReader(`{
        "model": "llama3.2:3b",
        "prompt": "Your prompt here",
        "stream": false
    }`),
)
```

## Resource Configuration

### Current Limits
- **Memory**: 2Gi request, 8Gi limit per replica
- **CPU**: 1 request, 4 limit per replica
- **Storage**: 20Gi persistent volume
- **Replicas**: 1-3 (auto-scaling)

### GPU Support (Future Enhancement)
To enable GPU support, you would need to:

1. Ensure your cluster has GPU nodes with NVIDIA device plugin
2. Update the deployment with GPU resources:
   ```yaml
   resources:
     limits:
       nvidia.com/gpu: 1
   ```
3. Use GPU-enabled Ollama image: `ollama/ollama:cuda`

## Security Considerations

- The Ollama service runs as root (required for model access)
- Network policies restrict access to within the cluster and specific ingress paths
- External access is controlled through ingress with TLS termination
- Consider adding authentication for production use

## Monitoring

The deployment includes standard Kubernetes monitoring through your existing Prometheus/Grafana setup. Key metrics to monitor:

- CPU and memory usage
- Request latency and error rates
- Model loading times
- Storage usage

## Troubleshooting

### Common Issues

1. **Models not loading**: Check the model pull job logs
   ```bash
   kubectl logs job/ollama-pull-models -n mozartpay
   ```

2. **High memory usage**: Consider using smaller models or adding more replicas
   ```bash
   kubectl top pods -n mozartpay -l app.kubernetes.io/component=ollama
   ```

3. **Connection refused**: Ensure the Ollama pod is running and ready
   ```bash
   kubectl get pods -n mozartpay -l app.kubernetes.io/component=ollama
   ```

### Logs Analysis
```bash
# Real-time logs
kubectl logs -n mozartpay deployment/ollama -f

# Previous deployment logs
kubectl logs -n mozartpay deployment/ollama --previous
```

## Backup and Recovery

### Model Backup
Models are stored in the persistent volume. To backup:

```bash
# Create a backup pod
kubectl run ollama-backup --image=busybox -n mozartpay --restart=Never -- \
  tar czf /backup/ollama-models.tar.gz -C /root/.ollama .

# Copy backup
kubectl cp mozartpay/ollama-backup:/backup/ollama-models.tar.gz ./ollama-backup.tar.gz
```

### Recovery
```bash
# Restore from backup
kubectl cp ./ollama-backup.tar.gz mozartpay/$(kubectl get pods -n mozartpay -l app.kubernetes.io/component=ollama -o jsonpath='{.items[0].metadata.name}'):/tmp/

# Extract in container
kubectl exec -n mozartpay deployment/ollama -- tar xzf /tmp/ollama-backup.tar.gz -C /root/.ollama
```

## Future Enhancements

1. **Model Versioning**: Implement model version control and rollback
2. **Model Caching**: Add intelligent model caching and preloading
3. **Multi-Model Support**: Configure for concurrent model serving
4. **Authentication**: Add API key or token-based authentication
5. **Metrics**: Export Ollama-specific metrics to Prometheus
6. **GPU Acceleration**: Enable GPU support for larger models
