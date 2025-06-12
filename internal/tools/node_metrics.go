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

func NewNodeMetricsTool(pool k8s.ClientPool) fxctx.Tool {
	schema := toolinput.NewToolInputSchema(
		toolinput.WithRequiredString("context", "Name of the Kubernetes context to use"),
		toolinput.WithString("node", "Name of the node to get resource usage for (if not provided, returns all nodes)"),
	)
	return fxctx.NewTool(
		&mcp.Tool{
			Name:        "get-k8s-node-metrics",
			Description: utils.Ptr("Get Kubernetes node metrics (CPU and memory usage) using the specified context"),
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

			// Check if specific node is requested
			k8sNode, _ := input.String("node")
			if k8sNode != "" {
				// Get metrics for specific node
				nodeMetrics, err := metricsClient.MetricsV1beta1().
					NodeMetricses().
					Get(ctx, k8sNode, metav1.GetOptions{})
				if err != nil {
					return errResponse(fmt.Errorf("failed to get node metrics: %w", err))
				}
				result = convertNodeMetricsToStruct(nodeMetrics)
			} else {
				// Get metrics for all nodes
				nodeMetricsList, err := metricsClient.MetricsV1beta1().
					NodeMetricses().
					List(ctx, metav1.ListOptions{})
				if err != nil {
					return errResponse(fmt.Errorf("failed to list node metrics: %w", err))
				}
				result = convertNodeMetricsListToStruct(nodeMetricsList)
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

// NodeMetrics provides a structured representation of node resource usage
type NodeMetrics struct {
	Name        string    `json:"name"`
	CPUCores    float64   `json:"cpu_cores"`
	MemoryBytes int64     `json:"memory_bytes"`
	Timestamp   time.Time `json:"timestamp"`
}

// NodeMetricsList provides a list of node metrics
type NodeMetricsList struct {
	Nodes []NodeMetrics `json:"nodes"`
}

// Convert NodeMetrics to our return struct
func convertNodeMetricsToStruct(nodeMetrics *metricsv1beta1.NodeMetrics) NodeMetrics {
	cpu := nodeMetrics.Usage.Cpu().MilliValue()  // MilliCores
	memory := nodeMetrics.Usage.Memory().Value() // Bytes

	return NodeMetrics{
		Name:        nodeMetrics.Name,
		CPUCores:    float64(cpu) / 1000.0,
		MemoryBytes: memory,
		Timestamp:   nodeMetrics.Timestamp.Time,
	}
}

// Convert NodeMetricsList to our return struct
func convertNodeMetricsListToStruct(nodeMetricsList *metricsv1beta1.NodeMetricsList) NodeMetricsList {
	nodes := make([]NodeMetrics, len(nodeMetricsList.Items))

	for i, nodeMetrics := range nodeMetricsList.Items {
		nodes[i] = convertNodeMetricsToStruct(&nodeMetrics)
	}

	return NodeMetricsList{
		Nodes: nodes,
	}
}
