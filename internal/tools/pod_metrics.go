package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/strowk/mcp-k8s-go/internal/k8s"
	"github.com/strowk/mcp-k8s-go/internal/utils"

	"github.com/strowk/foxy-contexts/pkg/fxctx"
	"github.com/strowk/foxy-contexts/pkg/mcp"
	"github.com/strowk/foxy-contexts/pkg/toolinput"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

func NewPodMetricsTool(pool k8s.ClientPool) fxctx.Tool {
	schema := toolinput.NewToolInputSchema(
		toolinput.WithRequiredString("context", "Name of the Kubernetes context to use"),
		toolinput.WithString("namespace", "Name of the namespace where the pod is located (if not provided, returns "+
			"metrics for pods from all namespaces)"),
		toolinput.WithString("pod", "Name of the pod to get resource usage for (if not provided, returns metrics for "+
			"all pods in the given namespace or all pods in the cluster if no namespace is provided)"),
	)
	return fxctx.NewTool(
		&mcp.Tool{
			Name:        "get-k8s-pod-metrics",
			Description: utils.Ptr("Get resource metrics (CPU and memory usage) for Kubernetes pods using specific context"),
			InputSchema: schema.GetMcpToolInputSchema(),
		},
		func(ctx context.Context, args map[string]interface{}) *mcp.CallToolResult {
			input, err := schema.Validate(args)
			if err != nil {
				return errResponse(fmt.Errorf("invalid input: %w", err))
			}

			k8sCtx, err := input.String("context")
			if err != nil {
				return errResponse(fmt.Errorf("invalid input: %w", err))
			}

			// Get the rest config from the clientset
			// We need this to create the metrics client
			kubeConfig := k8s.GetKubeConfigForContext(k8sCtx)
			config, err := kubeConfig.ClientConfig()
			if err != nil {
				return errResponse(fmt.Errorf("failed to get config: %w", err))
			}

			// Create metrics client
			metricsClient, err := metricsclientset.NewForConfig(config)
			if err != nil {
				return errResponse(fmt.Errorf("failed to create metrics client: %w", err))
			}

			var result interface{}

			// Get optional parameters
			k8sNamespace, _ := input.String("namespace")
			k8sPod, _ := input.String("pod")

			if k8sPod != "" && k8sNamespace == "" {
				// We require a namespace when a specific pod is given
				// NOTE: This is more strict than kubectl which will default to the context's ns or the default ns
				// and simply return an error if the given pod isn't found.
				return errResponse(fmt.Errorf("pod name specified but namespace is required when querying for a specific pod"))
			} else if k8sPod != "" && k8sNamespace != "" {
				// Get metrics for specific pod in specific namespace
				podMetrics, err := metricsClient.MetricsV1beta1().
					PodMetricses(k8sNamespace).
					Get(ctx, k8sPod, metav1.GetOptions{})
				if err != nil {
					return errResponse(fmt.Errorf("failed to get pod metrics: %w", err))
				}
				result = convertPodMetricsToStruct(podMetrics)
			} else if k8sNamespace != "" {
				// Get metrics for all pods in specific namespace
				podMetricsList, err := metricsClient.MetricsV1beta1().
					PodMetricses(k8sNamespace).
					List(ctx, metav1.ListOptions{})
				if err != nil {
					return errResponse(fmt.Errorf("failed to list pod metrics in namespace %s: %w", k8sNamespace, err))
				}
				result = convertPodMetricsListToStruct(podMetricsList)
			} else {
				// Get metrics for all pods across all namespaces
				podMetricsList, err := metricsClient.MetricsV1beta1().
					PodMetricses("").
					List(ctx, metav1.ListOptions{})
				if err != nil {
					return errResponse(fmt.Errorf("failed to list pod metrics across all namespaces: %w", err))
				}
				result = convertPodMetricsListToStruct(podMetricsList)
			}

			content, err := NewJsonContent(result)
			if err != nil {
				return errResponse(err)
			}

			return &mcp.CallToolResult{
				Content: []interface{}{content},
				IsError: utils.Ptr(false),
			}
		},
	)
}

// PodMetrics provides a structured representation of pod resource usage
type PodMetrics struct {
	Name        string             `json:"name"`
	Namespace   string             `json:"namespace"`
	CPUCores    float64            `json:"cpu_cores"`
	MemoryBytes int64              `json:"memory_bytes"`
	Timestamp   time.Time          `json:"timestamp"`
	Containers  []ContainerMetrics `json:"containers"`
}

// ContainerMetrics provides resource usage for a single container
type ContainerMetrics struct {
	Name        string  `json:"name"`
	CPUCores    float64 `json:"cpu_cores"`
	MemoryBytes int64   `json:"memory_bytes"`
}

// PodMetricsList provides a list of pod metrics
type PodMetricsList struct {
	Pods []PodMetrics `json:"pods"`
}

// Convert PodMetrics to our return struct and calculate total CPU and memory across all containers
func convertPodMetricsToStruct(podMetrics *metricsv1beta1.PodMetrics) PodMetrics {
	var totalCPU, totalMemory int64
	containers := make([]ContainerMetrics, len(podMetrics.Containers))

	for i, container := range podMetrics.Containers {
		cpu := container.Usage.Cpu().MilliValue()  // MilliCores
		memory := container.Usage.Memory().Value() // Bytes
		totalCPU += cpu
		totalMemory += memory

		containers[i] = ContainerMetrics{
			Name:        container.Name,
			CPUCores:    float64(cpu) / 1000.0,
			MemoryBytes: memory,
		}
	}

	return PodMetrics{
		Name:        podMetrics.Name,
		Namespace:   podMetrics.Namespace,
		CPUCores:    float64(totalCPU) / 1000.0,
		MemoryBytes: totalMemory,
		Timestamp:   podMetrics.Timestamp.Time,
		Containers:  containers,
	}
}

// Convert PodMetricsList to our return struct
func convertPodMetricsListToStruct(podMetricsList *metricsv1beta1.PodMetricsList) PodMetricsList {
	pods := make([]PodMetrics, len(podMetricsList.Items))

	for i, podMetrics := range podMetricsList.Items {
		pods[i] = convertPodMetricsToStruct(&podMetrics)
	}

	return PodMetricsList{
		Pods: pods,
	}
}
