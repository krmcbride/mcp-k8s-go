package pod

import (
	"fmt"
	"time"

	"github.com/strowk/mcp-k8s-go/internal/k8s/list_mapping"
	"github.com/strowk/mcp-k8s-go/internal/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// PodInList provides a structured representation of Pod information
type PodInList struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Restarts  int32  `json:"restarts"`
	Age       string `json:"age"`
	Node      string `json:"node"`
	PodIP     string `json:"pod_ip"`
}

func (p *PodInList) GetName() string {
	return p.Name
}

func (p *PodInList) GetNamespace() string {
	return p.Namespace
}

func NewPodInList(pod *corev1.Pod) *PodInList {
	// Calculate age
	age := time.Since(pod.CreationTimestamp.Time)

	// Count ready containers and total restarts
	readyCount := 0
	totalCount := len(pod.Status.ContainerStatuses)
	var totalRestarts int32

	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Ready {
			readyCount++
		}
		totalRestarts += containerStatus.RestartCount
	}

	// Format ready status as "ready/total"
	readyStatus := fmt.Sprintf("%d/%d", readyCount, totalCount)

	// Get pod phase/status
	status := string(pod.Status.Phase)
	if pod.Status.Reason != "" {
		status = pod.Status.Reason
	}

	// Handle special statuses
	if pod.DeletionTimestamp != nil {
		status = "Terminating"
	}

	return &PodInList{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Status:    status,
		Ready:     readyStatus,
		Restarts:  totalRestarts,
		Age:       utils.FormatAge(age),
		Node:      pod.Spec.NodeName,
		PodIP:     pod.Status.PodIP,
	}
}

func getPodListMapping() list_mapping.ListMapping {
	return func(u runtime.Unstructured) (list_mapping.ListContentItem, error) {
		pod := corev1.Pod{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(u.UnstructuredContent(), &pod, false)
		if err != nil {
			return nil, err
		}
		return NewPodInList(&pod), nil
	}
}

type listMappingResolver struct {
	list_mapping.ListMappingResolver
}

func (r *listMappingResolver) GetListMapping(gvk *schema.GroupVersionKind) list_mapping.ListMapping {
	if (gvk.Group == "core" || gvk.Group == "") && gvk.Version == "v1" && gvk.Kind == "Pod" {
		return getPodListMapping()
	}
	return nil
}

func NewListMappingResolver() list_mapping.ListMappingResolver {
	return &listMappingResolver{}
}
