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
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

const defaultRunbookBaseURL = "https://paperclip.inc/docs/operators/openclaw/runbooks"

// PrometheusRuleGVK returns the GroupVersionKind for PrometheusRule
func PrometheusRuleGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "PrometheusRule",
	}
}

// PrometheusRuleName returns the name of the PrometheusRule
func PrometheusRuleName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name + "-alerts"
}

// BuildPrometheusRule creates an unstructured PrometheusRule for the OpenClawInstance
func BuildPrometheusRule(instance *openclawv1alpha1.OpenClawInstance) *unstructured.Unstructured {
	labels := Labels(instance)

	// Add custom labels from spec
	prLabels := make(map[string]string)
	for k, v := range labels {
		prLabels[k] = v
	}
	if instance.Spec.Observability.Metrics.PrometheusRule != nil {
		for k, v := range instance.Spec.Observability.Metrics.PrometheusRule.Labels {
			prLabels[k] = v
		}
	}

	runbookBase := defaultRunbookBaseURL
	if instance.Spec.Observability.Metrics.PrometheusRule != nil &&
		instance.Spec.Observability.Metrics.PrometheusRule.RunbookBaseURL != "" {
		runbookBase = instance.Spec.Observability.Metrics.PrometheusRule.RunbookBaseURL
	}

	alerts := buildAlerts(instance, runbookBase)

	pr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "PrometheusRule",
			"metadata": map[string]interface{}{
				"name":      PrometheusRuleName(instance),
				"namespace": instance.Namespace,
				"labels":    toStringInterfaceMap(prLabels),
			},
			"spec": map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name":  "openclaw-operator",
						"rules": alerts,
					},
				},
			},
		},
	}

	return pr
}

func buildAlerts(instance *openclawv1alpha1.OpenClawInstance, runbookBase string) []interface{} {
	// Helper to quote a label value in PromQL (avoids sprintfQuotedString lint)
	q := func(s string) string { return `"` + s + `"` }
	name := instance.Name
	ns := instance.Namespace

	alerts := []interface{}{
		buildAlert(
			"OpenClawReconcileErrors",
			`sum(rate(openclaw_reconcile_total{result="error",instance=`+q(name)+`,namespace=`+q(ns)+`}[5m])) > 0`,
			"5m",
			"warning",
			"OpenClaw instance {{ $labels.instance }} in {{ $labels.namespace }} has reconciliation errors.",
			runbookBase,
		),
		buildAlert(
			"OpenClawInstanceDegraded",
			`openclaw_instance_phase{phase=~"Failed|Degraded",instance=`+q(name)+`,namespace=`+q(ns)+`} == 1`,
			"5m",
			"critical",
			"OpenClaw instance {{ $labels.instance }} in {{ $labels.namespace }} is in {{ $labels.phase }} phase.",
			runbookBase,
		),
		buildAlert(
			"OpenClawSlowReconciliation",
			`histogram_quantile(0.99, sum(rate(openclaw_reconcile_duration_seconds_bucket{instance=`+q(name)+`,namespace=`+q(ns)+`}[5m])) by (le)) > 30`,
			"5m",
			"warning",
			"OpenClaw instance {{ $labels.instance }} p99 reconciliation duration exceeds 30s.",
			runbookBase,
		),
		buildAlert(
			"OpenClawPodCrashLooping",
			`increase(kube_pod_container_status_restarts_total{namespace=`+q(ns)+`,pod=~`+q(name+"-.*")+`,container="openclaw"}[10m]) > 2`,
			"0m",
			"critical",
			"OpenClaw pod {{ $labels.pod }} is crash-looping (>2 restarts in 10m).",
			runbookBase,
		),
		buildAlert(
			"OpenClawPodOOMKilled",
			`kube_pod_container_status_last_terminated_reason{reason="OOMKilled",namespace=`+q(ns)+`,pod=~`+q(name+"-.*")+`,container="openclaw"} == 1`,
			"0m",
			"warning",
			"OpenClaw pod {{ $labels.pod }} was OOM killed. Consider increasing memory limits.",
			runbookBase,
		),
	}

	if IsPersistenceEnabled(instance) {
		claimMatcher := pvcClaimMatcher(instance)
		alerts = append(alerts, buildAlert(
			"OpenClawPVCNearlyFull",
			`(kubelet_volume_stats_used_bytes{namespace=`+q(ns)+`,`+claimMatcher+`} / kubelet_volume_stats_capacity_bytes{namespace=`+q(ns)+`,`+claimMatcher+`}) > 0.80`,
			"5m",
			"warning",
			"PVC for OpenClaw instance {{ $labels.persistentvolumeclaim }} is over 80% full.",
			runbookBase,
		))
	}

	return append(alerts, buildAlert(
		"OpenClawAutoUpdateRollback",
		`increase(openclaw_autoupdate_rollbacks_total{instance=`+q(name)+`,namespace=`+q(ns)+`}[1h]) > 0`,
		"0m",
		"warning",
		"OpenClaw instance {{ $labels.instance }} auto-update rolled back in the last hour.",
		runbookBase,
	))
}

func pvcClaimMatcher(instance *openclawv1alpha1.OpenClawInstance) string {
	if IsHPAEnabled(instance) {
		claimPattern := regexp.QuoteMeta("data-"+StatefulSetName(instance)+"-") + `[0-9]+`
		return `persistentvolumeclaim=~"` + claimPattern + `"`
	}

	claimName := PVCName(instance)
	if instance.Spec.Storage.Persistence.ExistingClaim != "" {
		claimName = instance.Spec.Storage.Persistence.ExistingClaim
	}
	return `persistentvolumeclaim="` + claimName + `"`
}

func buildAlert(alertName, expr, forDuration, severity, summary, runbookBase string) map[string]interface{} {
	return map[string]interface{}{
		"alert": alertName,
		"expr":  expr,
		"for":   forDuration,
		"labels": map[string]interface{}{
			"severity": severity,
		},
		"annotations": map[string]interface{}{
			"summary":     summary,
			"runbook_url": fmt.Sprintf("%s/%s", runbookBase, alertName),
		},
	}
}
