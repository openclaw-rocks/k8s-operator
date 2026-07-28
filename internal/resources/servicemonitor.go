/*
Copyright 2026 Paperclip Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// ServiceMonitorGVK returns the GroupVersionKind for ServiceMonitor
func ServiceMonitorGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "ServiceMonitor",
	}
}

// ServiceMonitorName returns the name of the ServiceMonitor
func ServiceMonitorName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name
}

// BuildServiceMonitor creates an unstructured ServiceMonitor for the OpenClawInstance
func BuildServiceMonitor(instance *openclawv1alpha1.OpenClawInstance) *unstructured.Unstructured {
	labels := Labels(instance)

	// Add custom labels from spec
	smLabels := make(map[string]string)
	for k, v := range labels {
		smLabels[k] = v
	}
	if instance.Spec.Observability.Metrics.ServiceMonitor != nil {
		for k, v := range instance.Spec.Observability.Metrics.ServiceMonitor.Labels {
			smLabels[k] = v
		}
	}

	interval := "30s"
	if instance.Spec.Observability.Metrics.ServiceMonitor != nil &&
		instance.Spec.Observability.Metrics.ServiceMonitor.Interval != "" {
		interval = instance.Spec.Observability.Metrics.ServiceMonitor.Interval
	}

	selectorLabels := SelectorLabels(instance)

	sm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "ServiceMonitor",
			"metadata": map[string]interface{}{
				"name":      ServiceMonitorName(instance),
				"namespace": instance.Namespace,
				"labels":    toStringInterfaceMap(smLabels),
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": toStringInterfaceMap(selectorLabels),
				},
				"endpoints": []interface{}{
					map[string]interface{}{
						"port":     MetricsPortName,
						"interval": interval,
						"path":     "/metrics",
					},
				},
			},
		},
	}

	return sm
}

func toStringInterfaceMap(m map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
