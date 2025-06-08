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

func NewPodTopTool(pool k8s.ClientPool) fxctx.Tool {
	schema := toolinput.NewToolInputSchema(
		toolinput.WithRequiredString("context", "Name of the Kubernetes context to use"),
		toolinput.WithRequiredString("namespace", "Name of the namespace where the pod is located"),
		toolinput.WithRequiredString("pod", "Name of the pod to get resource usage for"),
	)
	return fxctx.NewTool(
		&mcp.Tool{
			Name:        "get-k8s-pod-top",
			Description: utils.Ptr("Get resource usage (CPU and memory) for a Kubernetes pod using specific context in a specified namespace"),
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

			k8sNamespace, err := input.String("namespace")
			if err != nil {
				return errResponse(fmt.Errorf("invalid input: %w", err))
			}

			k8sPod, err := input.String("pod")
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

			// Get pod metrics
			podMetrics, err := metricsClient.MetricsV1beta1().
				PodMetricses(k8sNamespace).
				Get(ctx, k8sPod, metav1.GetOptions{})
			if err != nil {
				return errResponse(fmt.Errorf("failed to get pod metrics: %w", err))
			}

			// Convert metrics to structured data
			podTop := convertPodMetricsToStruct(podMetrics)

			content, err := NewJsonContent(podTop)
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

// PodTop provides a structured representation of pod resource usage
type PodTop struct {
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

// Convert PodMetrics to our return struct and calculate total CPU and memory across all containers
func convertPodMetricsToStruct(podMetrics *metricsv1beta1.PodMetrics) PodTop {
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

	return PodTop{
		Name:        podMetrics.Name,
		Namespace:   podMetrics.Namespace,
		CPUCores:    float64(totalCPU) / 1000.0,
		MemoryBytes: totalMemory,
		Timestamp:   podMetrics.Timestamp.Time,
		Containers:  containers,
	}
}
