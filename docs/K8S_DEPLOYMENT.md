# Kubernetes Deployment Guide

This project uses **Kustomize** to manage deployments across different stages (Staging, Production).

## 1. Directory Structure

- `k8s/base`: Common resource definitions (Deployments, Services, ConfigMaps).
- `k8s/overlays/staging`: Staging-specific overrides (prefixes, image tags).
- `k8s/overlays/production`: Production-specific overrides (replicas, higher resource limits).

## 2. Prerequisites

- A running Kubernetes cluster (Minikube, Kind, or managed cloud EKS/GKE).
- `kubectl` with Kustomize support (v1.14+).

## 3. How to Deploy

### Staging
```bash
kubectl apply -k k8s/overlays/staging
```

### Production
```bash
kubectl apply -k k8s/overlays/production
```

## 4. Key Infrastructure Notes

- **MySQL:** In the `base` manifest, MySQL uses an `emptyDir` for data. For a real staging/prod environment, you MUST update `k8s/base/mysql.yaml` to use a `PersistentVolumeClaim` (PVC) or use a managed database service.
- **Migrations:** The `migrate` Job runs as a standard K8s Job. Ensure it completes successfully before traffic hits the services.
- **CDC:** The `outbox-relay` is configured to use CDC mode. Ensure your MySQL deployment in K8s has binlogging enabled (included in the base manifest).

## 5. Health Checks
All microservices include:
- **Liveness Probes:** Restarts the container if the `/healthz` endpoint fails.
- **Readiness Probes:** Ensures the service is only added to the LoadBalancer once it is ready to receive traffic.
