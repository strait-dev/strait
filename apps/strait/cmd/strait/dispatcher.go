package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/compute"
	"strait/internal/config"
	"strait/internal/dispatcher"
)

// runDispatcher starts the process in dispatcher mode.
//
// Dispatcher mode does not run the API or worker. It starts a lightweight HTTP
// server that:
//   - Responds to /health for the Cloudflare LB health monitor
//   - Proxies all other requests to the Strait cluster with the lowest queue depth
//
// The cluster list is read from the cluster-registry ConfigMap in Kubernetes
// (DISPATCHER_CLUSTER_REGISTRY_CONFIGMAP / DISPATCHER_CLUSTER_REGISTRY_NAMESPACE).
// The ConfigMap is reloaded every DISPATCHER_REFRESH_INTERVAL (default 5s).
//
// This mode requires COMPUTE_RUNTIME=k8s so it can reach the K8s API to read
// the ConfigMap. K8S_KUBECONFIG and K8S_NAMESPACE are inherited from the usual
// config vars.
func runDispatcher(ctx context.Context, cfg *config.Config) error {
	slog.Info("starting dispatcher", "refresh_interval", cfg.DispatcherRefreshInterval)

	clientset, err := compute.BuildK8sClientset(cfg.K8sKubeconfig)
	if err != nil {
		return fmt.Errorf("dispatcher: build k8s clientset: %w", err)
	}

	registry := dispatcher.NewClusterRegistry(
		clientset,
		cfg.DispatcherClusterRegistryNamespace,
		cfg.DispatcherClusterRegistryConfigMap,
		slog.Default(),
	)

	// Initial load — fail fast if the ConfigMap is missing.
	if err := registry.Reload(ctx); err != nil {
		return fmt.Errorf("dispatcher: initial registry load: %w", err)
	}

	// Background refresh loop.
	go func() {
		interval := cfg.DispatcherRefreshInterval
		if interval <= 0 {
			interval = 5 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := registry.Reload(ctx); err != nil {
					slog.Warn("dispatcher: registry reload failed", "error", err)
				}
			}
		}
	}()

	d := dispatcher.New(registry, cfg.Port, slog.Default())
	return d.Run(ctx)
}
