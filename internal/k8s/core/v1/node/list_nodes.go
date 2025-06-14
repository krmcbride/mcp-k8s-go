package node

import (
	"strings"
	"time"

	"github.com/strowk/mcp-k8s-go/internal/k8s/list_mapping"
	"github.com/strowk/mcp-k8s-go/internal/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NodeInList provides a structured representation of Node information
type NodeInList struct {
	Name              string            `json:"name"`
	Status            string            `json:"status"`
	Roles             []string          `json:"roles"`
	Age               string            `json:"age"`
	Version           string            `json:"version"`
	InternalIP        string            `json:"internal_ip"`
	ExternalIP        string            `json:"external_ip"`
	OSImage           string            `json:"os_image"`
	KernelVersion     string            `json:"kernel_version"`
	ContainerRuntime  string            `json:"container_runtime"`
	CPUCapacity       string            `json:"cpu_capacity"`
	MemoryCapacity    string            `json:"memory_capacity"`
	StorageCapacity   string            `json:"storage_capacity"`
	PodsCapacity      string            `json:"pods_capacity"`
	CPUAllocatable    string            `json:"cpu_allocatable"`
	MemoryAllocatable string            `json:"memory_allocatable"`
	Taints            []string          `json:"taints"`
	Conditions        map[string]string `json:"conditions"`
}

func (n *NodeInList) GetName() string {
	return n.Name
}

func (n *NodeInList) GetNamespace() string {
	return ""
}

func NewNodeInList(node *corev1.Node) *NodeInList {
	// Calculate age
	age := time.Since(node.CreationTimestamp.Time)

	// Extract roles from labels
	roles := extractNodeRoles(node)

	// Get node status
	status := getNodeStatus(node)

	// Extract IP addresses
	internalIP, externalIP := extractNodeIPs(node)

	// Extract capacity and allocatable resources
	cpuCapacity := node.Status.Capacity[corev1.ResourceCPU]
	memoryCapacity := node.Status.Capacity[corev1.ResourceMemory]
	storageCapacity := node.Status.Capacity[corev1.ResourceEphemeralStorage]
	podsCapacity := node.Status.Capacity[corev1.ResourcePods]

	cpuAllocatable := node.Status.Allocatable[corev1.ResourceCPU]
	memoryAllocatable := node.Status.Allocatable[corev1.ResourceMemory]

	// Extract taints
	taints := extractTaints(node)

	// Extract conditions
	conditions := extractConditions(node)

	return &NodeInList{
		Name:              node.Name,
		Status:            status,
		Roles:             roles,
		Age:               utils.FormatAge(age),
		Version:           node.Status.NodeInfo.KubeletVersion,
		InternalIP:        internalIP,
		ExternalIP:        externalIP,
		OSImage:           node.Status.NodeInfo.OSImage,
		KernelVersion:     node.Status.NodeInfo.KernelVersion,
		ContainerRuntime:  node.Status.NodeInfo.ContainerRuntimeVersion,
		CPUCapacity:       formatResource(cpuCapacity),
		MemoryCapacity:    formatResource(memoryCapacity),
		StorageCapacity:   formatResource(storageCapacity),
		PodsCapacity:      formatResource(podsCapacity),
		CPUAllocatable:    formatResource(cpuAllocatable),
		MemoryAllocatable: formatResource(memoryAllocatable),
		Taints:            taints,
		Conditions:        conditions,
	}
}

func extractNodeRoles(node *corev1.Node) []string {
	roles := []string{}
	
	// Check for control-plane role
	if _, exists := node.Labels["node-role.kubernetes.io/control-plane"]; exists {
		roles = append(roles, "control-plane")
	}
	if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
		roles = append(roles, "master")
	}
	
	// Check for other roles
	for label := range node.Labels {
		if strings.HasPrefix(label, "node-role.kubernetes.io/") && 
		   label != "node-role.kubernetes.io/control-plane" && 
		   label != "node-role.kubernetes.io/master" {
			role := strings.TrimPrefix(label, "node-role.kubernetes.io/")
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	
	if len(roles) == 0 {
		roles = append(roles, "worker")
	}
	
	return roles
}

func getNodeStatus(node *corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return "Ready"
			} else {
				return "NotReady"
			}
		}
	}
	return "Unknown"
}

func extractNodeIPs(node *corev1.Node) (string, string) {
	var internalIP, externalIP string
	
	for _, address := range node.Status.Addresses {
		switch address.Type {
		case corev1.NodeInternalIP:
			internalIP = address.Address
		case corev1.NodeExternalIP:
			externalIP = address.Address
		}
	}
	
	return internalIP, externalIP
}

func extractTaints(node *corev1.Node) []string {
	taints := []string{}
	for _, taint := range node.Spec.Taints {
		taintStr := taint.Key + "=" + taint.Value + ":" + string(taint.Effect)
		taints = append(taints, taintStr)
	}
	return taints
}

func extractConditions(node *corev1.Node) map[string]string {
	conditions := make(map[string]string)
	for _, condition := range node.Status.Conditions {
		conditions[string(condition.Type)] = string(condition.Status)
	}
	return conditions
}

func formatResource(quantity resource.Quantity) string {
	if quantity.IsZero() {
		return "0"
	}
	return quantity.String()
}

func getNodeListMapping() list_mapping.ListMapping {
	return func(u runtime.Unstructured) (list_mapping.ListContentItem, error) {
		node := corev1.Node{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(u.UnstructuredContent(), &node, false)
		if err != nil {
			return nil, err
		}
		return NewNodeInList(&node), nil
	}
}

type listMappingResolver struct {
	list_mapping.ListMappingResolver
}

func (r *listMappingResolver) GetListMapping(gvk *schema.GroupVersionKind) list_mapping.ListMapping {
	if (gvk.Group == "core" || gvk.Group == "") && gvk.Version == "v1" && gvk.Kind == "Node" {
		return getNodeListMapping()
	}
	return nil
}

func NewListMappingResolver() list_mapping.ListMappingResolver {
	return &listMappingResolver{}
}