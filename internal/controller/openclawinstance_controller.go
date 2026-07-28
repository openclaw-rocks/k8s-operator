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

package controller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/registry"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
	"github.com/paperclipinc/openclaw-operator/internal/skillpacks"
)

const (
	// FinalizerName is the finalizer used by this controller
	FinalizerName = "openclaw.rocks/finalizer"

	// RequeueAfter is the default requeue interval
	RequeueAfter = 5 * time.Minute

	// ProvisioningRequeueAfter is the requeue interval used while waiting for
	// the StatefulSet's pod to become ready. It is shorter than RequeueAfter
	// so the Phase flips to Running promptly once the pod reports ready,
	// rather than waiting up to 5 minutes even though the StatefulSet watch
	// should already trigger a reconcile on status change.
	ProvisioningRequeueAfter = 10 * time.Second
)

// requeueError is a sentinel error used by reconcileResources to signal
// that a sub-step (like restore) needs to requeue with a specific Result.
type requeueError struct {
	Result ctrl.Result
}

func (e *requeueError) Error() string {
	return fmt.Sprintf("requeue after %v", e.Result.RequeueAfter)
}

// OpenClawInstanceReconciler reconciles a OpenClawInstance object
type OpenClawInstanceReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	OperatorNamespace string
	VersionResolver   *registry.Resolver
	SkillPackResolver *skillpacks.Resolver
}

// +kubebuilder:rbac:groups=openclaw.rocks,resources=openclawinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openclaw.rocks,resources=openclawinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openclaw.rocks,resources=openclawinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=openclaw.rocks,resources=openclawclusterdefaults,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *OpenClawInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileStart := time.Now()
	defer func() {
		reconcileDuration.WithLabelValues(req.Name, req.Namespace).Observe(time.Since(reconcileStart).Seconds())
	}()

	logger := log.FromContext(ctx)
	logger.Info("Reconciling OpenClawInstance")

	// Fetch the OpenClawInstance
	instance := &openclawv1alpha1.OpenClawInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("OpenClawInstance not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get OpenClawInstance")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDeleteWithBackup(ctx, instance)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(instance, FinalizerName) {
		logger.Info("Adding finalizer")
		controllerutil.AddFinalizer(instance, FinalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set initial phase if not set
	if instance.Status.Phase == "" {
		instance.Status.Phase = openclawv1alpha1.PhasePending
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Update phase to Provisioning
	if instance.Status.Phase == openclawv1alpha1.PhasePending {
		instance.Status.Phase = openclawv1alpha1.PhaseProvisioning
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}

		// Resolve "latest" to a concrete semver tag if auto-update is enabled
		if resolved, err := r.resolveInitialTag(ctx, instance); err != nil {
			return ctrl.Result{}, err
		} else if resolved {
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Snapshot status before mutations so we can skip no-op status updates
	savedStatus := instance.Status.DeepCopy()

	// If an auto-update is in progress, drive the state machine.
	// Pause the update state machine while suspended - pending version stays in status and resumes on unsuspend.
	if instance.Status.AutoUpdate.PendingVersion != "" && !instance.Spec.Suspended {
		result, err := r.reconcileAutoUpdate(ctx, instance)
		if err != nil {
			logger.Error(err, "Auto-update error (non-fatal)")
			r.Recorder.Event(instance, corev1.EventTypeWarning, "AutoUpdateError", err.Error())
		}
		if result.Requeue || result.RequeueAfter > 0 {
			if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return result, nil
		}
	}

	// Reconcile all resources
	if err := r.reconcileResources(ctx, instance); err != nil {
		// Check if this is a requeue signal (e.g., from restore in progress)
		if rqErr, ok := err.(*requeueError); ok {
			return rqErr.Result, nil
		}

		// A Kubernetes optimistic-lock conflict ("object has been modified") is a
		// benign, retryable controller-runtime race (commonly seen on resume when
		// StatefulSet/status are updated from a slightly stale resourceVersion),
		// not a workload failure. Requeue promptly without emitting a failed-looking
		// status or a ReconcileFailed event; the next reconcile runs against a fresh
		// resourceVersion and succeeds (issue #553).
		if res, ok := retryableConflictResult(err); ok {
			logger.V(1).Info("Reconcile hit an optimistic-lock conflict; requeueing without marking failed", "error", err.Error())
			reconcileTotal.WithLabelValues(instance.Name, instance.Namespace, "conflict").Inc()
			return res, nil
		}

		logger.Error(err, "Failed to reconcile resources")
		r.Recorder.Event(instance, corev1.EventTypeWarning, "ReconcileFailed", err.Error())

		// Update status to Failed
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    openclawv1alpha1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ReconcileFailed",
			Message: err.Error(),
		})
		instance.Status.Phase = openclawv1alpha1.PhaseFailed
		reconcileTotal.WithLabelValues(instance.Name, instance.Namespace, "error").Inc()
		updatePhaseMetric(instance.Name, instance.Namespace, instance.Status.Phase)
		if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}

		// Use shorter requeue for transient errors, longer for persistent ones
		requeueAfter := 30 * time.Second
		if instance.Status.Phase == openclawv1alpha1.PhaseFailed {
			// If already in failed state, back off more
			requeueAfter = 2 * time.Minute
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	// Handle suspended state: override phase and readiness
	if instance.Spec.Suspended {
		instance.Status.Phase = openclawv1alpha1.PhaseSuspended
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    openclawv1alpha1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "Suspended",
			Message: "Instance is suspended (spec.suspended=true), workload scaled to zero",
		})
		if instance.Status.ObservedGeneration != instance.Generation {
			instance.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
		}
		instance.Status.ObservedGeneration = instance.Generation

		statusChanged := !equality.Semantic.DeepEqual(&instance.Status, savedStatus)
		if statusChanged {
			if err := r.Status().Update(ctx, instance); err != nil {
				logger.Error(err, "Failed to update status")
				return ctrl.Result{}, err
			}
			r.Recorder.Event(instance, corev1.EventTypeNormal, "Suspended", "Instance suspended, workload scaled to zero")
		}
		reconcileTotal.WithLabelValues(instance.Name, instance.Namespace, "success").Inc()
		updatePhaseMetric(instance.Name, instance.Namespace, instance.Status.Phase)
		return ctrl.Result{RequeueAfter: RequeueAfter}, nil
	}

	// Determine phase based on condition health. The StatefulSet's pod
	// readiness takes precedence: downstream consumers (e.g. the hosted
	// proxy) treat Phase==Running as "safe to route to the instance
	// Service", so we must not report Running while the StatefulSet has
	// zero ready replicas - that Service would have zero ready endpoints.
	provisioningWait := false
	stsReadyCondition := meta.FindStatusCondition(instance.Status.Conditions, openclawv1alpha1.ConditionTypeStatefulSetReady)
	skillPacksCondition := meta.FindStatusCondition(instance.Status.Conditions, openclawv1alpha1.ConditionTypeSkillPacksReady)
	switch {
	case stsReadyCondition == nil || stsReadyCondition.Status != metav1.ConditionTrue:
		instance.Status.Phase = openclawv1alpha1.PhaseProvisioning
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    openclawv1alpha1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "StatefulSetNotReady",
			Message: "Waiting for the StatefulSet pod to become ready",
		})
		provisioningWait = true
	case skillPacksCondition != nil && skillPacksCondition.Status == metav1.ConditionFalse:
		instance.Status.Phase = openclawv1alpha1.PhaseDegraded
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    openclawv1alpha1.ConditionTypeReady,
			Status:  metav1.ConditionTrue,
			Reason:  "ReconcileSucceededDegraded",
			Message: "Resources reconciled but skill packs unavailable - instance running without skill packs",
		})
	default:
		instance.Status.Phase = openclawv1alpha1.PhaseRunning
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    openclawv1alpha1.ConditionTypeReady,
			Status:  metav1.ConditionTrue,
			Reason:  "ReconcileSucceeded",
			Message: "All resources reconciled successfully",
		})
	}
	if instance.Status.ObservedGeneration != instance.Generation {
		instance.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
	}
	instance.Status.ObservedGeneration = instance.Generation

	// Check for auto-updates (non-fatal — errors are logged and evented)
	autoUpdateResult, autoUpdateErr := r.reconcileAutoUpdate(ctx, instance)
	if autoUpdateErr != nil {
		logger.Error(autoUpdateErr, "Auto-update check failed (non-fatal)")
	}

	// Skip status update and event when nothing changed (avoids watch-triggered requeue loop)
	statusChanged := !equality.Semantic.DeepEqual(&instance.Status, savedStatus)
	if statusChanged {
		if err := r.Status().Update(ctx, instance); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(instance, corev1.EventTypeNormal, "ReconcileSucceeded", "All resources reconciled successfully")
	}

	reconcileTotal.WithLabelValues(instance.Name, instance.Namespace, "success").Inc()
	updatePhaseMetric(instance.Name, instance.Namespace, instance.Status.Phase)
	logger.Info("Reconciliation completed successfully")

	// If auto-update needs a requeue, use its interval if shorter. While
	// waiting for the StatefulSet's pod to become ready, requeue sooner than
	// the default interval so Phase flips to Running promptly (the owned-
	// StatefulSet watch should already trigger a reconcile on status change,
	// but this is a cheap belt-and-suspenders backstop).
	requeueAfter := RequeueAfter
	if provisioningWait {
		requeueAfter = ProvisioningRequeueAfter
	}
	if autoUpdateResult.Requeue {
		return autoUpdateResult, nil
	}
	if autoUpdateResult.RequeueAfter > 0 && autoUpdateResult.RequeueAfter < requeueAfter {
		requeueAfter = autoUpdateResult.RequeueAfter
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func updatePhaseMetric(name, namespace, currentPhase string) {
	phases := []string{"Pending", "Provisioning", "Running", "Degraded", "Failed", "Terminating", "BackingUp", "Restoring", "Updating", "Suspended"}
	for _, phase := range phases {
		val := float64(0)
		if phase == currentPhase {
			val = 1
		}
		instancePhase.WithLabelValues(name, namespace, phase).Set(val)
	}
}

// applyClusterDefaults fetches the cluster-scoped OpenClawClusterDefaults
// singleton (must be named "cluster") and merges its spec into the in-memory
// instance. The merged fields are only used for rendering owned resources;
// the user's stored spec is never overwritten in etcd.
//
// Cluster defaults are optional: if no singleton exists, the instance is
// returned unchanged. If a defaults CR exists under a non-singleton name, it
// is ignored and a warning event is emitted on the instance so platform
// operators can fix the name.
func (r *OpenClawInstanceReconciler) applyClusterDefaults(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	logger := log.FromContext(ctx)

	defaults := &openclawv1alpha1.OpenClawClusterDefaults{}
	err := r.Get(ctx, types.NamespacedName{Name: openclawv1alpha1.ClusterDefaultsSingletonName}, defaults)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get OpenClawClusterDefaults/%s: %w", openclawv1alpha1.ClusterDefaultsSingletonName, err)
	}

	merged := resources.ApplyClusterDefaults(instance, defaults)
	instance.Spec = merged.Spec
	logger.V(1).Info("applied OpenClawClusterDefaults", "generation", defaults.Generation)
	return nil
}

// reconcileResources reconciles all managed resources
func (r *OpenClawInstanceReconciler) reconcileResources(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	logger := log.FromContext(ctx)

	// Merge the cluster-wide defaults singleton into the instance spec for
	// rendering purposes. The merged spec is never written back to etcd;
	// we only mutate the in-memory copy returned by r.Get so downstream
	// builders see the defaulted values. Per-instance fields always win.
	if err := r.applyClusterDefaults(ctx, instance); err != nil {
		return fmt.Errorf("failed to apply cluster defaults: %w", err)
	}

	// 1. Reconcile RBAC (ServiceAccount, Role, RoleBinding)
	if err := r.reconcileRBAC(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile RBAC: %w", err)
	}
	logger.V(1).Info("RBAC reconciled")

	// 2. Reconcile NetworkPolicy
	if err := r.reconcileNetworkPolicy(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile NetworkPolicy: %w", err)
	}
	logger.V(1).Info("NetworkPolicy reconciled")

	// 2b. Reconcile gateway token Secret (must precede ConfigMap + StatefulSet)
	gatewayToken, err := r.reconcileGatewayTokenSecret(ctx, instance)
	if err != nil {
		return fmt.Errorf("failed to reconcile gateway token secret: %w", err)
	}
	logger.V(1).Info("Gateway token secret reconciled")

	// 2c. Reconcile Tailscale state Secret (must precede StatefulSet)
	if instance.Spec.Tailscale.Enabled {
		err = r.reconcileTailscaleStateSecret(ctx, instance)
		if err != nil {
			return fmt.Errorf("failed to reconcile Tailscale state secret: %w", err)
		}
		logger.V(1).Info("Tailscale state secret reconciled")
	}

	// 2d. Resolve skill packs from GitHub (non-blocking - failures degrade but don't block provisioning)
	// Top-level spec.skills pack: entries resolve into the default workspace;
	// additionalWorkspaces[].skills pack: entries resolve per workspace (#568).
	var skillPacks *resources.ResolvedSkillPacks
	packNames := resources.ExtractPackSkills(instance.Spec.Skills)
	type workspacePacks struct {
		wsName string
		packs  []string
	}
	var wsPackNames []workspacePacks
	totalPacks := len(packNames)
	if instance.Spec.Workspace != nil {
		for i := range instance.Spec.Workspace.AdditionalWorkspaces {
			aw := &instance.Spec.Workspace.AdditionalWorkspaces[i]
			if packs := resources.ExtractPackSkills(aw.Skills); len(packs) > 0 {
				wsPackNames = append(wsPackNames, workspacePacks{wsName: aw.Name, packs: packs})
				totalPacks += len(packs)
			}
		}
	}
	if totalPacks > 0 && r.SkillPackResolver != nil {
		var resolveErrs []string
		resolvedCount := 0
		if len(packNames) > 0 {
			var resolved *resources.ResolvedSkillPacks
			resolved, err = r.SkillPackResolver.Resolve(ctx, packNames)
			if err != nil {
				logger.Error(err, "Failed to resolve skill packs, continuing without them", "packs", packNames)
				resolveErrs = append(resolveErrs, err.Error())
			} else {
				skillPacks = resolved
				resolvedCount += len(packNames)
			}
		}
		for _, wp := range wsPackNames {
			var resolved *resources.ResolvedSkillPacks
			resolved, err = r.SkillPackResolver.Resolve(ctx, wp.packs)
			if err != nil {
				logger.Error(err, "Failed to resolve workspace skill packs, continuing without them",
					"workspace", wp.wsName, "packs", wp.packs)
				resolveErrs = append(resolveErrs, fmt.Sprintf("workspace %q: %v", wp.wsName, err))
				continue
			}
			if skillPacks == nil {
				skillPacks = &resources.ResolvedSkillPacks{}
			}
			if skillPacks.Workspaces == nil {
				skillPacks.Workspaces = make(map[string]*resources.ResolvedSkillPacks)
			}
			skillPacks.Workspaces[wp.wsName] = resolved
			resolvedCount += len(wp.packs)
		}
		if len(resolveErrs) > 0 {
			r.Recorder.Event(instance, corev1.EventTypeWarning, "SkillPackResolutionFailed",
				fmt.Sprintf("Failed to resolve skill packs: %s. Instance will start without them.", strings.Join(resolveErrs, "; ")))
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               openclawv1alpha1.ConditionTypeSkillPacksReady,
				Status:             metav1.ConditionFalse,
				Reason:             "ResolutionFailed",
				Message:            fmt.Sprintf("Failed to resolve skill packs: %s", strings.Join(resolveErrs, "; ")),
				ObservedGeneration: instance.Generation,
			})
		} else {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               openclawv1alpha1.ConditionTypeSkillPacksReady,
				Status:             metav1.ConditionTrue,
				Reason:             "Resolved",
				Message:            fmt.Sprintf("Successfully resolved %d skill pack(s)", resolvedCount),
				ObservedGeneration: instance.Generation,
			})
			logger.V(1).Info("Skill packs resolved", "packs", packNames, "workspacePacks", len(wsPackNames))
		}
	}

	// 3. Reconcile ConfigMap (always - enrichment pipeline runs on all config sources)
	err = r.reconcileConfigMap(ctx, instance, gatewayToken, skillPacks)
	if err != nil {
		return fmt.Errorf("failed to reconcile ConfigMap: %w", err)
	}
	logger.V(1).Info("ConfigMap reconciled")

	if resources.HasGatewayBindConflict(instance) {
		r.Recorder.Event(instance, corev1.EventTypeWarning, "GatewayBindConflict",
			"gateway.enabled is false but config sets gateway.bind to loopback - the pod will be unreachable because no proxy is running on the external interface")
	}

	// 3b. Reconcile Workspace ConfigMap (seed files for workspace)
	wsFiles, err := r.reconcileWorkspaceConfigMap(ctx, instance, skillPacks)
	if err != nil {
		return fmt.Errorf("failed to reconcile Workspace ConfigMap: %w", err)
	}
	logger.V(1).Info("Workspace ConfigMap reconciled")

	// 4. Reconcile PVC
	if err := r.reconcilePVC(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile PVC: %w", err)
	}
	logger.V(1).Info("PVC reconciled")

	// 4a. Reconcile Chromium PVC (if persistence is enabled)
	if err := r.reconcileChromiumPVC(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile Chromium PVC: %w", err)
	}
	logger.V(1).Info("Chromium PVC reconciled")

	// 4b. Restore from backup if spec.restoreFrom is set (must happen after PVC, before StatefulSet)
	if result, done, err := r.reconcileRestore(ctx, instance); !done {
		if err != nil {
			return fmt.Errorf("failed to reconcile restore: %w", err)
		}
		// Restore in progress — return a sentinel error that Reconcile interprets as "requeue with result"
		// We store the result for the caller to use
		return &requeueError{Result: result}
	}
	logger.V(1).Info("Restore reconciled")

	// 5. Reconcile PodDisruptionBudget
	if err := r.reconcilePDB(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile PodDisruptionBudget: %w", err)
	}
	logger.V(1).Info("PodDisruptionBudget reconciled")

	// 5b. Reconcile HorizontalPodAutoscaler
	if err := r.reconcileHPA(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile HPA: %w", err)
	}
	logger.V(1).Info("HPA reconciled")

	// 6. Migrate Deployment → StatefulSet (if legacy Deployment exists), then reconcile StatefulSet
	if err := r.migrateDeploymentToStatefulSet(ctx, instance); err != nil {
		return fmt.Errorf("failed to migrate Deployment to StatefulSet: %w", err)
	}
	if err := r.reconcileStatefulSet(ctx, instance, gatewayToken, skillPacks, wsFiles); err != nil {
		return fmt.Errorf("failed to reconcile StatefulSet: %w", err)
	}
	logger.V(1).Info("StatefulSet reconciled")

	// 6b. Reconcile periodic backup CronJob (after StatefulSet so pod affinity labels exist)
	if err := r.reconcileBackupCronJob(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile backup CronJob: %w", err)
	}
	logger.V(1).Info("Backup CronJob reconciled")

	// 7. Reconcile Service
	if err := r.reconcileService(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile Service: %w", err)
	}
	logger.V(1).Info("Service reconciled")

	// 7b. Reconcile Chromium CDP headless Service (if chromium is enabled)
	if err := r.reconcileChromiumCDPService(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile Chromium CDP Service: %w", err)
	}

	// 8. Reconcile Ingress (if enabled)
	if err := r.reconcileIngress(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile Ingress: %w", err)
	}
	logger.V(1).Info("Ingress reconciled")

	// 8b. Reconcile HTTPRoute (Gateway API, if enabled)
	if err := r.reconcileHTTPRoute(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile HTTPRoute: %w", err)
	}
	logger.V(1).Info("HTTPRoute reconciled")

	// 9. Reconcile ServiceMonitor (if enabled)
	if err := r.reconcileServiceMonitor(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile ServiceMonitor: %w", err)
	}
	logger.V(1).Info("ServiceMonitor reconciled")

	// 10. Reconcile PrometheusRule (if enabled)
	if err := r.reconcilePrometheusRule(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile PrometheusRule: %w", err)
	}
	logger.V(1).Info("PrometheusRule reconciled")

	// 11. Reconcile Grafana Dashboards (if enabled)
	if err := r.reconcileGrafanaDashboards(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile Grafana dashboards: %w", err)
	}
	logger.V(1).Info("Grafana dashboards reconciled")

	return nil
}

// reconcileRBAC reconciles ServiceAccount, Role, and RoleBinding
func (r *OpenClawInstanceReconciler) reconcileRBAC(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	// Check if we should create a ServiceAccount
	createSA := instance.Spec.Security.RBAC.CreateServiceAccount == nil || *instance.Spec.Security.RBAC.CreateServiceAccount

	if createSA {
		// Reconcile ServiceAccount
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resources.ServiceAccountName(instance),
				Namespace: instance.Namespace,
			},
		}
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
			desired := resources.BuildServiceAccount(instance)
			sa.Labels = mergeStringMap(sa.Labels, desired.Labels)
			sa.Annotations = mergeStringMap(sa.Annotations, desired.Annotations)
			sa.AutomountServiceAccountToken = desired.AutomountServiceAccountToken
			return controllerutil.SetControllerReference(instance, sa, r.Scheme)
		}); err != nil {
			return err
		}
		instance.Status.ManagedResources.ServiceAccount = sa.Name

		// Reconcile Role
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resources.RoleName(instance),
				Namespace: instance.Namespace,
			},
		}
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
			desired := resources.BuildRole(instance)
			role.Labels = mergeStringMap(role.Labels, desired.Labels)
			role.Rules = desired.Rules
			return controllerutil.SetControllerReference(instance, role, r.Scheme)
		}); err != nil {
			return err
		}
		instance.Status.ManagedResources.Role = role.Name

		// Reconcile RoleBinding
		roleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resources.RoleBindingName(instance),
				Namespace: instance.Namespace,
			},
		}
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, roleBinding, func() error {
			desired := resources.BuildRoleBinding(instance)
			roleBinding.Labels = mergeStringMap(roleBinding.Labels, desired.Labels)
			roleBinding.RoleRef = desired.RoleRef
			roleBinding.Subjects = desired.Subjects
			return controllerutil.SetControllerReference(instance, roleBinding, r.Scheme)
		}); err != nil {
			return err
		}
		instance.Status.ManagedResources.RoleBinding = roleBinding.Name
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    openclawv1alpha1.ConditionTypeRBACReady,
		Status:  metav1.ConditionTrue,
		Reason:  "RBACCreated",
		Message: "RBAC resources created successfully",
	})

	return nil
}

// reconcileNetworkPolicy reconciles the NetworkPolicy
func (r *OpenClawInstanceReconciler) reconcileNetworkPolicy(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	// Check if NetworkPolicy is enabled
	enabled := instance.Spec.Security.NetworkPolicy.Enabled == nil || *instance.Spec.Security.NetworkPolicy.Enabled

	if !enabled {
		// Delete existing NetworkPolicy if it exists
		np := &networkingv1.NetworkPolicy{}
		np.Name = resources.NetworkPolicyName(instance)
		np.Namespace = instance.Namespace
		if err := r.Delete(ctx, np); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		instance.Status.ManagedResources.NetworkPolicy = ""
		return nil
	}

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.NetworkPolicyName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		desired := resources.BuildNetworkPolicy(instance)
		np.Labels = mergeStringMap(np.Labels, desired.Labels)
		np.Spec = desired.Spec
		return controllerutil.SetControllerReference(instance, np, r.Scheme)
	}); err != nil {
		return err
	}
	instance.Status.ManagedResources.NetworkPolicy = np.Name

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    openclawv1alpha1.ConditionTypeNetworkPolicyReady,
		Status:  metav1.ConditionTrue,
		Reason:  "NetworkPolicyCreated",
		Message: "NetworkPolicy created successfully",
	})

	return nil
}

// isGatewayAuthTrustedProxy checks whether the instance's config (inline or
// external ConfigMap) sets gateway.auth.mode to "trusted-proxy". This mode is
// mutually exclusive with token-based auth, so the operator must not inject
// gateway token env vars or config keys when it is active.
func (r *OpenClawInstanceReconciler) isGatewayAuthTrustedProxy(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) bool {
	if instance.Spec.Config.ConfigMapRef != nil {
		ref := instance.Spec.Config.ConfigMapRef
		externalCM := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      ref.Name,
		}, externalCM); err != nil {
			return false
		}
		key := ref.Key
		if key == "" {
			key = "openclaw.json"
		}
		data, ok := externalCM.Data[key]
		if !ok {
			return false
		}
		return resources.IsGatewayAuthTrustedProxy([]byte(data))
	}
	if instance.Spec.Config.Raw != nil {
		return resources.IsGatewayAuthTrustedProxy(instance.Spec.Config.Raw.Raw)
	}
	return false
}

// reconcileGatewayTokenSecret ensures a gateway token Secret exists for the instance.
// If spec.gateway.existingSecret is set, the operator uses that Secret instead of
// auto-generating one. Otherwise, a random 32-byte hex token is generated and stored.
// The token is used to configure gateway.auth.mode=token so that Bonjour/mDNS
// pairing (unusable in k8s) is bypassed.
func (r *OpenClawInstanceReconciler) reconcileGatewayTokenSecret(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) (string, error) {
	// If the user provides their own secret, look it up and return its token
	if instance.Spec.Gateway.ExistingSecret != "" {
		existing := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: instance.Spec.Gateway.ExistingSecret, Namespace: instance.Namespace}, existing); err != nil {
			if apierrors.IsNotFound(err) {
				return "", fmt.Errorf("gateway.existingSecret %q not found", instance.Spec.Gateway.ExistingSecret)
			}
			return "", fmt.Errorf("failed to get gateway existing secret: %w", err)
		}
		instance.Status.ManagedResources.GatewayTokenSecret = existing.Name
		if tok, ok := existing.Data[resources.GatewayTokenSecretKey]; ok {
			return string(tok), nil
		}
		return "", fmt.Errorf("gateway.existingSecret %q missing key %q", instance.Spec.Gateway.ExistingSecret, resources.GatewayTokenSecretKey)
	}

	secretName := resources.GatewayTokenSecretName(instance)

	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: instance.Namespace}, existing)
	if err == nil {
		// Secret exists — return the stored token
		instance.Status.ManagedResources.GatewayTokenSecret = existing.Name
		if tok, ok := existing.Data[resources.GatewayTokenSecretKey]; ok {
			return string(tok), nil
		}
		return "", nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to get gateway token secret: %w", err)
	}

	// Generate a random 32-byte hex token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate gateway token: %w", err)
	}
	tokenHex := hex.EncodeToString(tokenBytes)

	// Create the Secret via CreateOrUpdate (handles race conditions)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		desired := resources.BuildGatewayTokenSecret(instance, tokenHex)
		secret.Labels = mergeStringMap(secret.Labels, desired.Labels)
		// Only set data if this is a new Secret (don't overwrite user edits)
		if secret.Data == nil {
			secret.Data = desired.Data
		}
		return controllerutil.SetControllerReference(instance, secret, r.Scheme)
	}); err != nil {
		return "", fmt.Errorf("failed to create gateway token secret: %w", err)
	}

	instance.Status.ManagedResources.GatewayTokenSecret = secret.Name
	r.Recorder.Event(instance, corev1.EventTypeNormal, "GatewayTokenCreated", "Auto-generated gateway authentication token")

	return tokenHex, nil
}

// reconcileTailscaleStateSecret ensures an empty Secret exists for Tailscale to
// persist node identity and TLS certificate state. The containerboot process
// reads and writes state to this Secret via the Kubernetes API (TS_KUBE_SECRET).
// The operator pre-creates the Secret so that the pod's ServiceAccount only needs
// get/update/patch (not create) permissions, keeping RBAC minimal.
func (r *OpenClawInstanceReconciler) reconcileTailscaleStateSecret(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.TailscaleStateSecretName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		desired := resources.BuildTailscaleStateSecret(instance)
		secret.Labels = mergeStringMap(secret.Labels, desired.Labels)
		// Do not overwrite Data - containerboot manages the content
		return controllerutil.SetControllerReference(instance, secret, r.Scheme)
	}); err != nil {
		return fmt.Errorf("failed to reconcile Tailscale state secret: %w", err)
	}
	instance.Status.ManagedResources.TailscaleStateSecret = secret.Name
	return nil
}

// reconcileConfigMap reconciles the operator-managed ConfigMap for openclaw.json.
// It always creates the enriched ConfigMap regardless of config source (raw,
// configMapRef, or none). When configMapRef is set, the external ConfigMap is
// read and its content is used as the base for the enrichment pipeline.
func (r *OpenClawInstanceReconciler) reconcileConfigMap(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance, gatewayToken string, skillPacks *resources.ResolvedSkillPacks) error {
	var desired *corev1.ConfigMap

	if instance.Spec.Config.ConfigMapRef != nil {
		// Read the user's external ConfigMap
		ref := instance.Spec.Config.ConfigMapRef
		externalCM := &corev1.ConfigMap{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: instance.Namespace,
			Name:      ref.Name,
		}, externalCM); err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    openclawv1alpha1.ConditionTypeConfigValid,
				Status:  metav1.ConditionFalse,
				Reason:  "ConfigMapNotFound",
				Message: fmt.Sprintf("External ConfigMap %q not found: %v", ref.Name, err),
			})
			return fmt.Errorf("external ConfigMap %q not found: %w", ref.Name, err)
		}

		key := ref.Key
		if key == "" {
			key = "openclaw.json"
		}
		data, ok := externalCM.Data[key]
		if !ok {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    openclawv1alpha1.ConditionTypeConfigValid,
				Status:  metav1.ConditionFalse,
				Reason:  "ConfigMapKeyNotFound",
				Message: fmt.Sprintf("Key %q not found in ConfigMap %q", key, ref.Name),
			})
			return fmt.Errorf("key %q not found in ConfigMap %q", key, ref.Name)
		}

		desired = resources.BuildConfigMapFromBytes(instance, []byte(data), gatewayToken, skillPacks)
	} else {
		desired = resources.BuildConfigMap(instance, gatewayToken, skillPacks)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.ConfigMapName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = mergeStringMap(cm.Labels, desired.Labels)
		cm.Data = desired.Data
		return controllerutil.SetControllerReference(instance, cm, r.Scheme)
	}); err != nil {
		return err
	}
	instance.Status.ManagedResources.ConfigMap = cm.Name

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    openclawv1alpha1.ConditionTypeConfigValid,
		Status:  metav1.ConditionTrue,
		Reason:  "ConfigMapCreated",
		Message: "ConfigMap created successfully",
	})

	return nil
}

// reconcileWorkspaceConfigMap reconciles the ConfigMap containing workspace seed files.
// If the instance has no workspace files, any existing workspace ConfigMap is cleaned up.
// Returns the resolved external workspace files so callers (e.g. reconcileStatefulSet)
// can use them for config hash calculation and init script generation.
// resolvedWorkspaceFiles holds the resolved external files for default and additional workspaces.
type resolvedWorkspaceFiles struct {
	// defaultFiles are the resolved contents of spec.workspace.configMapRef.
	defaultFiles map[string]string
	// additionalFiles maps workspace name to resolved configMapRef contents.
	additionalFiles map[string]map[string]string
}

func (r *OpenClawInstanceReconciler) reconcileWorkspaceConfigMap(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance, skillPacks *resources.ResolvedSkillPacks) (*resolvedWorkspaceFiles, error) {
	logger := log.FromContext(ctx)
	resolved := &resolvedWorkspaceFiles{}
	hasConfigMapRef := false
	degraded := false

	// Resolve external workspace ConfigMap if referenced
	if instance.Spec.Workspace != nil && instance.Spec.Workspace.ConfigMapRef != nil {
		hasConfigMapRef = true
		ref := instance.Spec.Workspace.ConfigMapRef
		extCM := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: instance.Namespace}, extCM); err != nil {
			if apierrors.IsNotFound(err) {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               openclawv1alpha1.ConditionTypeWorkspaceReady,
					Status:             metav1.ConditionFalse,
					Reason:             "ConfigMapNotFound",
					Message:            fmt.Sprintf("Workspace ConfigMap %q not found", ref.Name),
					ObservedGeneration: instance.Generation,
				})
				r.Recorder.Eventf(instance, corev1.EventTypeWarning, "WorkspaceConfigMapNotFound",
					"Workspace ConfigMap %q referenced by spec.workspace.configMapRef not found", ref.Name)
				logger.Info("Workspace configMapRef not found, continuing without default workspace files", "configMap", ref.Name)
				degraded = true
			} else {
				return nil, fmt.Errorf("fetching workspace configMapRef %q: %w", ref.Name, err)
			}
		} else {
			resolved.defaultFiles = extCM.Data
			// Validate all keys from the external ConfigMap
			for key := range extCM.Data {
				vErr := resources.ValidateWorkspaceFilename(key)
				if vErr == nil {
					continue
				}
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               openclawv1alpha1.ConditionTypeWorkspaceReady,
					Status:             metav1.ConditionFalse,
					Reason:             "InvalidFilename",
					Message:            fmt.Sprintf("Workspace ConfigMap %q key %q: %v", ref.Name, key, vErr),
					ObservedGeneration: instance.Generation,
				})
				r.Recorder.Eventf(instance, corev1.EventTypeWarning, "InvalidWorkspaceFilename",
					"Workspace ConfigMap %q key %q: %v", ref.Name, key, vErr)
				logger.Info("Workspace configMapRef has invalid filename, continuing without default workspace files", "configMap", ref.Name, "key", key)
				resolved.defaultFiles = nil
				degraded = true
				break
			}
			if resolved.defaultFiles != nil {
				logger.V(1).Info("Resolved external workspace ConfigMap", "configMap", ref.Name, "keys", len(resolved.defaultFiles))
			}
		}
	}

	// Resolve additional workspace ConfigMaps
	if instance.Spec.Workspace != nil {
		for i := range instance.Spec.Workspace.AdditionalWorkspaces {
			aw := &instance.Spec.Workspace.AdditionalWorkspaces[i]
			if aw.ConfigMapRef == nil {
				continue
			}
			hasConfigMapRef = true
			extCM := &corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: aw.ConfigMapRef.Name, Namespace: instance.Namespace}, extCM); err != nil {
				if apierrors.IsNotFound(err) {
					meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
						Type:               openclawv1alpha1.ConditionTypeWorkspaceReady,
						Status:             metav1.ConditionFalse,
						Reason:             "ConfigMapNotFound",
						Message:            fmt.Sprintf("Additional workspace %q ConfigMap %q not found", aw.Name, aw.ConfigMapRef.Name),
						ObservedGeneration: instance.Generation,
					})
					r.Recorder.Eventf(instance, corev1.EventTypeWarning, "WorkspaceConfigMapNotFound",
						"Additional workspace %q ConfigMap %q not found", aw.Name, aw.ConfigMapRef.Name)
					logger.Info("Additional workspace configMapRef not found, skipping", "workspace", aw.Name, "configMap", aw.ConfigMapRef.Name)
					degraded = true
					continue
				}
				return nil, fmt.Errorf("fetching additional workspace %q configMapRef %q: %w", aw.Name, aw.ConfigMapRef.Name, err)
			}

			// Validate all keys
			valid := true
			for key := range extCM.Data {
				vErr := resources.ValidateWorkspaceFilename(key)
				if vErr == nil {
					continue
				}
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               openclawv1alpha1.ConditionTypeWorkspaceReady,
					Status:             metav1.ConditionFalse,
					Reason:             "InvalidFilename",
					Message:            fmt.Sprintf("Additional workspace %q ConfigMap %q key %q: %v", aw.Name, aw.ConfigMapRef.Name, key, vErr),
					ObservedGeneration: instance.Generation,
				})
				r.Recorder.Eventf(instance, corev1.EventTypeWarning, "InvalidWorkspaceFilename",
					"Additional workspace %q ConfigMap %q key %q: %v", aw.Name, aw.ConfigMapRef.Name, key, vErr)
				logger.Info("Additional workspace configMapRef has invalid filename, skipping", "workspace", aw.Name, "configMap", aw.ConfigMapRef.Name, "key", key)
				degraded = true
				valid = false
				break
			}
			if !valid {
				continue
			}

			if resolved.additionalFiles == nil {
				resolved.additionalFiles = make(map[string]map[string]string)
			}
			resolved.additionalFiles[aw.Name] = extCM.Data
			logger.V(1).Info("Resolved additional workspace ConfigMap", "workspace", aw.Name, "configMap", aw.ConfigMapRef.Name, "keys", len(extCM.Data))
		}
	}

	// Set WorkspaceReady=True when any configMapRef is used and all resolved successfully
	if hasConfigMapRef && !degraded {
		totalFiles := len(resolved.defaultFiles)
		for _, files := range resolved.additionalFiles {
			totalFiles += len(files)
		}
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               openclawv1alpha1.ConditionTypeWorkspaceReady,
			Status:             metav1.ConditionTrue,
			Reason:             "Resolved",
			Message:            fmt.Sprintf("All workspace ConfigMaps resolved with %d total file(s)", totalFiles),
			ObservedGeneration: instance.Generation,
		})
	}

	desired := resources.BuildWorkspaceConfigMap(instance, resolved.defaultFiles, resolved.additionalFiles, skillPacks)

	if desired == nil {
		// No workspace files - clean up existing ConfigMap if present
		existing := &corev1.ConfigMap{}
		existing.Name = resources.WorkspaceConfigMapName(instance)
		existing.Namespace = instance.Namespace
		if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		return resolved, nil
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.WorkspaceConfigMapName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = mergeStringMap(cm.Labels, desired.Labels)
		cm.Data = desired.Data
		return controllerutil.SetControllerReference(instance, cm, r.Scheme)
	}); err != nil {
		return nil, err
	}

	return resolved, nil
}

// reconcilePVC reconciles the PersistentVolumeClaim
func (r *OpenClawInstanceReconciler) reconcilePVC(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	// Check if persistence is enabled
	if !resources.IsPersistenceEnabled(instance) {
		instance.Status.ManagedResources.PVC = ""
		return nil
	}

	// When HPA is enabled, VolumeClaimTemplates on the StatefulSet handle
	// per-replica PVCs - skip creating the standalone PVC.
	if resources.IsHPAEnabled(instance) {
		// Log when existingClaim is set but ignored due to HPA (config choice, not operational issue)
		if instance.Spec.Storage.Persistence.ExistingClaim != "" {
			log.FromContext(ctx).V(1).Info("existingClaim ignored when autoScaling is enabled - each replica gets its own PVC via VolumeClaimTemplates",
				"existingClaim", instance.Spec.Storage.Persistence.ExistingClaim)
		}
		// Warn if a standalone PVC exists that is now orphaned by the switch to VCTs.
		// Only check when status still references the PVC to avoid a needless API call every reconcile.
		if instance.Status.ManagedResources.PVC != "" {
			orphanedPVC := &corev1.PersistentVolumeClaim{}
			pvcName := resources.PVCName(instance)
			if err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}, orphanedPVC); err == nil {
				r.Recorder.Eventf(instance, corev1.EventTypeWarning, "OrphanedPVC",
					"Standalone PVC %q is no longer used - per-replica PVCs are now managed by StatefulSet VolumeClaimTemplates. Delete the orphaned PVC manually if no longer needed.",
					pvcName)
			}
			instance.Status.ManagedResources.PVC = ""
		}
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               openclawv1alpha1.ConditionTypeStorageReady,
			Status:             metav1.ConditionTrue,
			Reason:             "ManagedByVolumeClaimTemplates",
			Message:            "Per-replica PVCs managed by StatefulSet VolumeClaimTemplates",
			ObservedGeneration: instance.Generation,
		})
		return nil
	}

	// Check if using existing claim
	if instance.Spec.Storage.Persistence.ExistingClaim != "" {
		existing := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, types.NamespacedName{Name: instance.Spec.Storage.Persistence.ExistingClaim, Namespace: instance.Namespace}, existing); err != nil {
			if apierrors.IsNotFound(err) {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               openclawv1alpha1.ConditionTypeStorageReady,
					Status:             metav1.ConditionFalse,
					Reason:             "ExistingClaimNotFound",
					Message:            fmt.Sprintf("Existing PVC %q not found", instance.Spec.Storage.Persistence.ExistingClaim),
					ObservedGeneration: instance.Generation,
				})
				return fmt.Errorf("existing PVC %q not found", instance.Spec.Storage.Persistence.ExistingClaim)
			}
			return err
		}
		instance.Status.ManagedResources.PVC = instance.Spec.Storage.Persistence.ExistingClaim
		return nil
	}

	pvc := resources.BuildPVC(instance)
	if err := controllerutil.SetControllerReference(instance, pvc, r.Scheme); err != nil {
		return err
	}

	// PVCs are immutable after creation, so we only create if not exists
	existing := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(pvc), existing); err != nil {
		if apierrors.IsNotFound(err) {
			if createErr := r.Create(ctx, pvc); createErr != nil {
				return createErr
			}
		} else {
			return err
		}
	}

	instance.Status.ManagedResources.PVC = pvc.Name

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    openclawv1alpha1.ConditionTypeStorageReady,
		Status:  metav1.ConditionTrue,
		Reason:  "PVCCreated",
		Message: "PersistentVolumeClaim created successfully",
	})

	return nil
}

// reconcileChromiumPVC reconciles the Chromium browser profile PersistentVolumeClaim
func (r *OpenClawInstanceReconciler) reconcileChromiumPVC(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	if !instance.Spec.Chromium.Enabled || !instance.Spec.Chromium.Persistence.Enabled {
		// Clean up managed Chromium PVC if persistence was disabled
		if instance.Status.ManagedResources.ChromiumPVC != "" &&
			instance.Spec.Chromium.Persistence.ExistingClaim == "" {
			pvc := &corev1.PersistentVolumeClaim{}
			pvc.Name = resources.ChromiumPVCName(instance)
			pvc.Namespace = instance.Namespace
			if err := r.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		instance.Status.ManagedResources.ChromiumPVC = ""
		return nil
	}

	// Using an existing claim - just track it in status
	if instance.Spec.Chromium.Persistence.ExistingClaim != "" {
		instance.Status.ManagedResources.ChromiumPVC = instance.Spec.Chromium.Persistence.ExistingClaim
		return nil
	}

	pvc := resources.BuildChromiumPVC(instance)
	if err := controllerutil.SetControllerReference(instance, pvc, r.Scheme); err != nil {
		return err
	}

	// PVCs are immutable after creation, so we only create if not exists
	existing := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(pvc), existing); err != nil {
		if apierrors.IsNotFound(err) {
			if createErr := r.Create(ctx, pvc); createErr != nil {
				return createErr
			}
		} else {
			return err
		}
	}

	instance.Status.ManagedResources.ChromiumPVC = pvc.Name
	return nil
}

// reconcilePDB reconciles the PodDisruptionBudget
func (r *OpenClawInstanceReconciler) reconcilePDB(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	// Check if PDB is enabled
	enabled := instance.Spec.Availability.PodDisruptionBudget == nil ||
		instance.Spec.Availability.PodDisruptionBudget.Enabled == nil ||
		*instance.Spec.Availability.PodDisruptionBudget.Enabled

	if !enabled {
		// Delete existing PDB if it exists
		pdb := &policyv1.PodDisruptionBudget{}
		pdb.Name = resources.PDBName(instance)
		pdb.Namespace = instance.Namespace
		if err := r.Delete(ctx, pdb); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		instance.Status.ManagedResources.PodDisruptionBudget = ""
		return nil
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.PDBName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, pdb, func() error {
		desired := resources.BuildPDB(instance)
		pdb.Labels = mergeStringMap(pdb.Labels, desired.Labels)
		pdb.Spec = desired.Spec
		return controllerutil.SetControllerReference(instance, pdb, r.Scheme)
	}); err != nil {
		return err
	}
	instance.Status.ManagedResources.PodDisruptionBudget = pdb.Name

	return nil
}

// reconcileHPA reconciles the HorizontalPodAutoscaler
func (r *OpenClawInstanceReconciler) reconcileHPA(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	if !resources.IsHPAEnabled(instance) {
		// Delete existing HPA if it exists
		hpa := &autoscalingv2.HorizontalPodAutoscaler{}
		hpa.Name = resources.HPAName(instance)
		hpa.Namespace = instance.Namespace
		if err := r.Delete(ctx, hpa); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		instance.Status.ManagedResources.HorizontalPodAutoscaler = ""
		return nil
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.HPAName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, hpa, func() error {
		desired := resources.BuildHPA(instance)
		hpa.Labels = mergeStringMap(hpa.Labels, desired.Labels)
		hpa.Spec = desired.Spec
		return controllerutil.SetControllerReference(instance, hpa, r.Scheme)
	}); err != nil {
		return err
	}
	instance.Status.ManagedResources.HorizontalPodAutoscaler = hpa.Name

	return nil
}

// migrateDeploymentToStatefulSet detects and deletes a legacy Deployment so
// the reconciler can create the replacement StatefulSet. This is a one-time
// migration step — once the Deployment is gone, this function is a no-op.
func (r *OpenClawInstanceReconciler) migrateDeploymentToStatefulSet(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	logger := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      resources.DeploymentName(instance),
		Namespace: instance.Namespace,
	}, deployment)
	if apierrors.IsNotFound(err) {
		return nil // already migrated
	}
	if err != nil {
		return err
	}

	// Safety check: only delete Deployments we own
	owned := false
	for _, ref := range deployment.OwnerReferences {
		if ref.UID == instance.UID {
			owned = true
			break
		}
	}
	if !owned {
		logger.Info("Deployment exists but is not owned by this instance, skipping migration",
			"deployment", deployment.Name)
		return nil
	}

	logger.Info("Migrating from Deployment to StatefulSet, deleting legacy Deployment",
		"deployment", deployment.Name)
	if err := r.Delete(ctx, deployment); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Clear legacy status fields
	instance.Status.ManagedResources.Deployment = ""
	meta.RemoveStatusCondition(&instance.Status.Conditions, openclawv1alpha1.ConditionTypeDeploymentReady)

	r.Recorder.Event(instance, corev1.EventTypeNormal, "MigrationDeploymentDeleted",
		"Legacy Deployment deleted, StatefulSet will be created")

	return nil
}

// reconcileStatefulSet reconciles the StatefulSet
func (r *OpenClawInstanceReconciler) reconcileStatefulSet(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance, gatewayToken string, skillPacks *resources.ResolvedSkillPacks, wsFiles *resolvedWorkspaceFiles) error {
	// Compute secret hash for rollout trigger on secret rotation
	secretHash, missingSecrets, err := r.computeSecretHash(ctx, instance)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to compute secret hash (non-fatal, using empty hash)")
		secretHash = ""
	}

	// Set SecretsReady condition based on missing secrets
	if len(missingSecrets) > 0 {
		for _, name := range missingSecrets {
			r.Recorder.Eventf(instance, corev1.EventTypeWarning, "SecretNotFound",
				"Secret %q referenced in envFrom is not found", name)
		}
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    openclawv1alpha1.ConditionTypeSecretsReady,
			Status:  metav1.ConditionFalse,
			Reason:  "SecretsMissing",
			Message: fmt.Sprintf("Missing secrets: %s", strings.Join(missingSecrets, ", ")),
		})
	} else {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    openclawv1alpha1.ConditionTypeSecretsReady,
			Status:  metav1.ConditionTrue,
			Reason:  "AllSecretsFound",
			Message: "All referenced secrets exist",
		})
	}

	// Compute gateway token secret name once for both VCT-change detection and CreateOrUpdate.
	// trusted-proxy mode is mutually exclusive with token auth - skip injecting the
	// OPENCLAW_GATEWAY_TOKEN env var when trusted-proxy is configured.
	var gwSecretName string
	if gatewayToken != "" && !r.isGatewayAuthTrustedProxy(ctx, instance) {
		if instance.Spec.Gateway.ExistingSecret != "" {
			gwSecretName = instance.Spec.Gateway.ExistingSecret
		} else {
			gwSecretName = resources.GatewayTokenSecretName(instance)
		}
	}

	// Build the desired StatefulSet once and reuse for both VCT comparison
	// and the CreateOrUpdate mutate func.
	desired := resources.BuildStatefulSet(instance, gwSecretName, skillPacks, wsFiles.defaultFiles, wsFiles.additionalFiles)
	resources.NormalizeStatefulSet(desired)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.StatefulSetName(instance),
			Namespace: instance.Namespace,
		},
	}

	// VolumeClaimTemplates are immutable on existing StatefulSets. Detect
	// transitions (e.g. enabling/disabling HPA with persistence) and
	// delete+recreate the StatefulSet when VCTs need to change.
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err == nil {
		if !resources.VolumeClaimTemplatesEqual(sts.Spec.VolumeClaimTemplates, desired.Spec.VolumeClaimTemplates) {
			log.FromContext(ctx).Info("VolumeClaimTemplates changed, recreating StatefulSet")
			if err := r.Client.Delete(ctx, sts); err != nil {
				return fmt.Errorf("deleting StatefulSet for VCT change: %w", err)
			}
			// Returning an error triggers exponential backoff; the next reconcile recreates the StatefulSet
			return fmt.Errorf("StatefulSet deleted for VolumeClaimTemplate change, will recreate on next reconcile")
		}
	}

	// Reset sts to a clean object for CreateOrUpdate (the Get above may have populated it)
	sts = &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.StatefulSetName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		sts.Labels = mergeStringMap(sts.Labels, desired.Labels)
		// Preserve current replica count when HPA manages scaling
		existingReplicas := sts.Spec.Replicas
		sts.Spec = desired.Spec
		if resources.IsHPAEnabled(instance) && !instance.Spec.Suspended && existingReplicas != nil {
			sts.Spec.Replicas = existingReplicas
		}
		// Inject secret hash annotation to trigger rollout on secret rotation
		if secretHash != "" {
			if sts.Spec.Template.Annotations == nil {
				sts.Spec.Template.Annotations = make(map[string]string)
			}
			sts.Spec.Template.Annotations["openclaw.rocks/secret-hash"] = secretHash
		}
		return controllerutil.SetControllerReference(instance, sts, r.Scheme)
	}); err != nil {
		return err
	}
	instance.Status.ManagedResources.StatefulSet = sts.Name

	// Check StatefulSet status
	var ready bool
	var stsCondStatus metav1.ConditionStatus
	var stsCondReason, stsCondMessage string

	if instance.Spec.Suspended {
		// Suspended: desired=0 replicas. Ready once all pods are terminated.
		ready = sts.Status.Replicas == 0
		if ready {
			stsCondStatus = metav1.ConditionTrue
			stsCondReason = "StatefulSetSuspended"
			stsCondMessage = "StatefulSet scaled to zero (instance suspended)"
		} else {
			stsCondStatus = metav1.ConditionFalse
			stsCondReason = "StatefulSetScalingDown"
			stsCondMessage = "StatefulSet is scaling down for suspension"
		}
	} else {
		ready = sts.Status.ReadyReplicas > 0
		if ready {
			stsCondStatus = metav1.ConditionTrue
			stsCondReason = "StatefulSetReady"
			stsCondMessage = "StatefulSet is ready"
		} else {
			stsCondStatus = metav1.ConditionFalse
			stsCondReason = "StatefulSetNotReady"
			stsCondMessage = "StatefulSet is not ready yet"
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    openclawv1alpha1.ConditionTypeStatefulSetReady,
		Status:  stsCondStatus,
		Reason:  stsCondReason,
		Message: stsCondMessage,
	})

	// Update instance readiness metric (0 when suspended - instance is not serving traffic)
	readyVal := float64(0)
	if ready && !instance.Spec.Suspended {
		readyVal = 1
	}
	instanceReady.WithLabelValues(instance.Name, instance.Namespace).Set(readyVal)

	// Update instance info metric
	instanceInfo.WithLabelValues(
		instance.Name,
		instance.Namespace,
		resources.GetImageTag(instance),
		resources.GetImage(instance),
	).Set(1)

	return nil
}

// reconcileService reconciles the Service
func (r *OpenClawInstanceReconciler) reconcileService(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.ServiceName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		desired := resources.BuildService(instance)
		service.Labels = mergeStringMap(service.Labels, desired.Labels)
		service.Annotations = mergeStringMap(service.Annotations, desired.Annotations)
		// Preserve ClusterIP — it is assigned by the API server and immutable
		clusterIP := service.Spec.ClusterIP
		clusterIPs := service.Spec.ClusterIPs
		service.Spec = desired.Spec
		service.Spec.ClusterIP = clusterIP
		service.Spec.ClusterIPs = clusterIPs
		return controllerutil.SetControllerReference(instance, service, r.Scheme)
	}); err != nil {
		return err
	}
	instance.Status.ManagedResources.Service = service.Name

	// Update endpoint in status
	instance.Status.GatewayEndpoint = fmt.Sprintf("%s.%s.svc:%d", service.Name, service.Namespace, resources.GatewayPort)
	instance.Status.CanvasEndpoint = fmt.Sprintf("%s.%s.svc:%d", service.Name, service.Namespace, resources.CanvasPort)

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    openclawv1alpha1.ConditionTypeServiceReady,
		Status:  metav1.ConditionTrue,
		Reason:  "ServiceCreated",
		Message: "Service created successfully",
	})

	return nil
}

// reconcileChromiumCDPService reconciles the headless Service used for the
// Chromium CDP endpoint. When chromium is disabled, the Service is deleted.
func (r *OpenClawInstanceReconciler) reconcileChromiumCDPService(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	svc := &corev1.Service{}
	svc.Name = resources.ChromiumCDPServiceName(instance)
	svc.Namespace = instance.Namespace

	if !instance.Spec.Chromium.Enabled {
		if err := r.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		desired := resources.BuildChromiumCDPService(instance)
		svc.Labels = mergeStringMap(svc.Labels, desired.Labels)
		svc.Spec = desired.Spec
		return controllerutil.SetControllerReference(instance, svc, r.Scheme)
	}); err != nil {
		return err
	}

	return nil
}

// reconcileIngress reconciles the Ingress and its supporting resources (basic auth Secret, Traefik Middleware).
func (r *OpenClawInstanceReconciler) reconcileIngress(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	if !instance.Spec.Networking.Ingress.Enabled {
		// Delete existing Ingress if it exists
		ing := &networkingv1.Ingress{}
		ing.Name = resources.IngressName(instance)
		ing.Namespace = instance.Namespace
		if err := r.Delete(ctx, ing); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	// Reconcile Basic Auth Secret (before Ingress so the annotation reference is valid)
	if err := r.reconcileBasicAuthSecret(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile basic auth secret: %w", err)
	}

	// Reconcile Traefik BasicAuth Middleware (no-op if not Traefik or basic auth disabled)
	if err := r.reconcileTraefikBasicAuthMiddleware(ctx, instance); err != nil {
		// Non-fatal: Traefik CRD may not be installed; log a warning but continue
		logger := log.FromContext(ctx)
		logger.Info("Could not reconcile Traefik BasicAuth Middleware (CRD may not be installed)", "error", err.Error())
	}

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.IngressName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, ingress, func() error {
		desired := resources.BuildIngress(instance)
		ingress.Labels = mergeStringMap(ingress.Labels, desired.Labels)
		ingress.Annotations = mergeStringMap(ingress.Annotations, desired.Annotations)
		ingress.Spec = desired.Spec
		return controllerutil.SetControllerReference(instance, ingress, r.Scheme)
	}); err != nil {
		return err
	}

	return nil
}

// reconcileHTTPRoute reconciles a Gateway API HTTPRoute (gateway.networking.k8s.io/v1).
// It is built as an unstructured object so the Gateway API Go module is not a build
// dependency. When disabled (or unset), any existing HTTPRoute is deleted. If the
// Gateway API CRDs are not installed, reconciliation is skipped silently.
func (r *OpenClawInstanceReconciler) reconcileHTTPRoute(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	enabled := instance.Spec.Networking.HTTPRoute != nil && instance.Spec.Networking.HTTPRoute.Enabled

	if !enabled {
		// Cleanup: delete existing HTTPRoute if it exists
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(resources.HTTPRouteGVK())
		existing.SetName(resources.HTTPRouteName(instance))
		existing.SetNamespace(instance.Namespace)
		if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			return err
		}
		instance.Status.ManagedResources.HTTPRoute = ""
		meta.RemoveStatusCondition(&instance.Status.Conditions, openclawv1alpha1.ConditionTypeHTTPRouteReady)
		return nil
	}

	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(resources.HTTPRouteGVK())
	route.SetName(resources.HTTPRouteName(instance))
	route.SetNamespace(instance.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
		desired := resources.BuildHTTPRoute(instance)

		if spec, ok := desired.Object["spec"]; ok {
			route.Object["spec"] = spec
		}
		route.SetLabels(desired.GetLabels())
		route.SetAnnotations(desired.GetAnnotations())

		ownerRef := metav1.OwnerReference{
			APIVersion: instance.APIVersion,
			Kind:       instance.Kind,
			Name:       instance.Name,
			UID:        instance.UID,
			Controller: resources.Ptr(true),
		}
		route.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
		return nil
	})
	if meta.IsNoMatchError(err) {
		// Gateway API CRDs not installed - skip silently
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    openclawv1alpha1.ConditionTypeHTTPRouteReady,
			Status:  metav1.ConditionFalse,
			Reason:  "GatewayAPINotInstalled",
			Message: "Gateway API CRDs (gateway.networking.k8s.io) are not installed",
		})
		return nil
	}
	if err != nil {
		return err
	}

	instance.Status.ManagedResources.HTTPRoute = route.GetName()
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    openclawv1alpha1.ConditionTypeHTTPRouteReady,
		Status:  metav1.ConditionTrue,
		Reason:  "HTTPRouteReconciled",
		Message: "Gateway API HTTPRoute reconciled successfully",
	})
	r.Recorder.Event(instance, corev1.EventTypeNormal, "HTTPRouteReconciled", "Gateway API HTTPRoute reconciled successfully")
	return nil
}

// reconcileBasicAuthSecret ensures the htpasswd Secret for Ingress Basic Auth exists.
// If spec.networking.ingress.security.basicAuth.existingSecret is set, no secret is created.
// Otherwise a random 20-byte password is generated once and stored in a managed Secret.
func (r *OpenClawInstanceReconciler) reconcileBasicAuthSecret(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	ba := instance.Spec.Networking.Ingress.Security.BasicAuth
	if ba == nil {
		return nil
	}
	enabled := ba.Enabled == nil || *ba.Enabled
	if !enabled {
		return nil
	}
	// User provided their own secret - nothing to generate
	if ba.ExistingSecret != "" {
		instance.Status.ManagedResources.BasicAuthSecret = ba.ExistingSecret
		return nil
	}

	secretName := resources.BasicAuthSecretName(instance)
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: instance.Namespace}, existing)
	if err == nil {
		instance.Status.ManagedResources.BasicAuthSecret = existing.Name
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get basic auth secret: %w", err)
	}

	// Generate a random 20-byte password
	pwdBytes := make([]byte, 20)
	if _, err := rand.Read(pwdBytes); err != nil {
		return fmt.Errorf("failed to generate basic auth password: %w", err)
	}
	password := hex.EncodeToString(pwdBytes)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		desired := resources.BuildBasicAuthSecret(instance, password)
		secret.Labels = mergeStringMap(secret.Labels, desired.Labels)
		if secret.Data == nil {
			secret.Data = desired.Data
		}
		return controllerutil.SetControllerReference(instance, secret, r.Scheme)
	}); err != nil {
		return fmt.Errorf("failed to create basic auth secret: %w", err)
	}

	instance.Status.ManagedResources.BasicAuthSecret = secret.Name
	r.Recorder.Event(instance, corev1.EventTypeNormal, "BasicAuthSecretCreated",
		"Auto-generated Ingress Basic Auth htpasswd Secret")
	return nil
}

// reconcileTraefikBasicAuthMiddleware creates a Traefik Middleware CRD instance for BasicAuth
// when the ingress class is Traefik and basic auth is enabled.
// Uses unstructured so the operator doesn't require the Traefik CRDs to be installed to build/run.
func (r *OpenClawInstanceReconciler) reconcileTraefikBasicAuthMiddleware(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	ba := instance.Spec.Networking.Ingress.Security.BasicAuth
	if ba == nil {
		return nil
	}
	enabled := ba.Enabled == nil || *ba.Enabled
	if !enabled {
		return nil
	}
	if resources.DetectIngressProvider(instance.Spec.Networking.Ingress.ClassName) != resources.IngressProviderTraefik {
		return nil
	}

	secretName := resources.BasicAuthSecretName(instance)
	if ba.ExistingSecret != "" {
		secretName = ba.ExistingSecret
	}

	middlewareName := instance.Name + "-basic-auth"
	mw := &unstructured.Unstructured{}
	mw.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "traefik.io",
		Version: "v1alpha1",
		Kind:    "Middleware",
	})
	mw.SetName(middlewareName)
	mw.SetNamespace(instance.Namespace)

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, mw, func() error {
		mw.Object["spec"] = map[string]interface{}{
			"basicAuth": map[string]interface{}{
				"secret": secretName,
			},
		}
		labels := resources.Labels(instance)
		mw.SetLabels(labels)
		return controllerutil.SetControllerReference(instance, mw, r.Scheme)
	}); err != nil {
		return err
	}
	return nil
}

// reconcileServiceMonitor reconciles the ServiceMonitor for Prometheus
func (r *OpenClawInstanceReconciler) reconcileServiceMonitor(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	// Check if ServiceMonitor is enabled
	if instance.Spec.Observability.Metrics.ServiceMonitor == nil ||
		instance.Spec.Observability.Metrics.ServiceMonitor.Enabled == nil ||
		!*instance.Spec.Observability.Metrics.ServiceMonitor.Enabled {
		return nil
	}

	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(resources.ServiceMonitorGVK())
	sm.SetName(resources.ServiceMonitorName(instance))
	sm.SetNamespace(instance.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sm, func() error {
		desired := resources.BuildServiceMonitor(instance)

		// Copy spec from desired into existing
		if spec, ok := desired.Object["spec"]; ok {
			sm.Object["spec"] = spec
		}
		sm.SetLabels(desired.GetLabels())

		// Set owner reference
		ownerRef := metav1.OwnerReference{
			APIVersion: instance.APIVersion,
			Kind:       instance.Kind,
			Name:       instance.Name,
			UID:        instance.UID,
			Controller: resources.Ptr(true),
		}
		sm.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
		return nil
	})
	return err
}

// reconcilePrometheusRule reconciles the PrometheusRule for alerting
func (r *OpenClawInstanceReconciler) reconcilePrometheusRule(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	prEnabled := instance.Spec.Observability.Metrics.PrometheusRule != nil &&
		instance.Spec.Observability.Metrics.PrometheusRule.Enabled != nil &&
		*instance.Spec.Observability.Metrics.PrometheusRule.Enabled

	if !prEnabled {
		// Cleanup: delete existing PrometheusRule if it exists
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(resources.PrometheusRuleGVK())
		existing.SetName(resources.PrometheusRuleName(instance))
		existing.SetNamespace(instance.Namespace)
		if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			return err
		}
		instance.Status.ManagedResources.PrometheusRule = ""
		return nil
	}

	pr := &unstructured.Unstructured{}
	pr.SetGroupVersionKind(resources.PrometheusRuleGVK())
	pr.SetName(resources.PrometheusRuleName(instance))
	pr.SetNamespace(instance.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pr, func() error {
		desired := resources.BuildPrometheusRule(instance)

		if spec, ok := desired.Object["spec"]; ok {
			pr.Object["spec"] = spec
		}
		pr.SetLabels(desired.GetLabels())

		ownerRef := metav1.OwnerReference{
			APIVersion: instance.APIVersion,
			Kind:       instance.Kind,
			Name:       instance.Name,
			UID:        instance.UID,
			Controller: resources.Ptr(true),
		}
		pr.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
		return nil
	})
	if meta.IsNoMatchError(err) {
		// PrometheusRule CRD not installed - skip silently
		return nil
	}
	if err != nil {
		return err
	}

	instance.Status.ManagedResources.PrometheusRule = pr.GetName()
	r.Recorder.Event(instance, corev1.EventTypeNormal, "PrometheusRuleReconciled", "PrometheusRule reconciled successfully")
	return nil
}

// reconcileGrafanaDashboards reconciles Grafana dashboard ConfigMaps
func (r *OpenClawInstanceReconciler) reconcileGrafanaDashboards(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	dashEnabled := instance.Spec.Observability.Metrics.GrafanaDashboard != nil &&
		instance.Spec.Observability.Metrics.GrafanaDashboard.Enabled != nil &&
		*instance.Spec.Observability.Metrics.GrafanaDashboard.Enabled

	if !dashEnabled {
		// Cleanup: delete existing dashboard ConfigMaps
		for _, name := range []string{
			resources.GrafanaDashboardOperatorName(instance),
			resources.GrafanaDashboardInstanceName(instance),
		} {
			existing := &corev1.ConfigMap{}
			existing.Name = name
			existing.Namespace = instance.Namespace
			if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		instance.Status.ManagedResources.GrafanaDashboardOperator = ""
		instance.Status.ManagedResources.GrafanaDashboardInstance = ""
		return nil
	}

	// Operator overview dashboard
	opCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.GrafanaDashboardOperatorName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, opCM, func() error {
		desired := resources.BuildGrafanaDashboardOperator(instance)
		opCM.Labels = mergeStringMap(opCM.Labels, desired.Labels)
		opCM.Annotations = mergeStringMap(opCM.Annotations, desired.Annotations)
		opCM.Data = desired.Data
		return controllerutil.SetControllerReference(instance, opCM, r.Scheme)
	}); err != nil {
		return err
	}
	instance.Status.ManagedResources.GrafanaDashboardOperator = opCM.Name

	// Instance detail dashboard
	instCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.GrafanaDashboardInstanceName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, instCM, func() error {
		desired := resources.BuildGrafanaDashboardInstance(instance)
		instCM.Labels = mergeStringMap(instCM.Labels, desired.Labels)
		instCM.Annotations = mergeStringMap(instCM.Annotations, desired.Annotations)
		instCM.Data = desired.Data
		return controllerutil.SetControllerReference(instance, instCM, r.Scheme)
	}); err != nil {
		return err
	}
	instance.Status.ManagedResources.GrafanaDashboardInstance = instCM.Name

	r.Recorder.Event(instance, corev1.EventTypeNormal, "GrafanaDashboardsReconciled", "Grafana dashboards reconciled successfully")
	return nil
}

// reconcileDelete is superseded by reconcileDeleteWithBackup in backup.go

// computeSecretHash reads all Secrets referenced via envFrom[].secretRef and
// computes a deterministic hash of their data. This hash is injected as a pod
// annotation so that secret rotations trigger a rolling restart.
// Returns the hash, a list of missing secret names, and any error.
func (r *OpenClawInstanceReconciler) computeSecretHash(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) (hash string, missingSecrets []string, err error) {
	var secretNames []string
	for _, ef := range instance.Spec.EnvFrom {
		if ef.SecretRef != nil {
			secretNames = append(secretNames, ef.SecretRef.Name)
		}
	}
	// Include the gateway token Secret so rotations trigger a pod rollout
	var gwSecretName string
	if instance.Spec.Gateway.ExistingSecret != "" {
		gwSecretName = instance.Spec.Gateway.ExistingSecret
	} else {
		gwSecretName = resources.GatewayTokenSecretName(instance)
	}
	secretNames = append(secretNames, gwSecretName)

	// Include the Tailscale auth key Secret so rotations trigger a pod rollout
	if instance.Spec.Tailscale.Enabled && instance.Spec.Tailscale.AuthKeySecretRef != nil {
		secretNames = append(secretNames, instance.Spec.Tailscale.AuthKeySecretRef.Name)
	}

	if len(secretNames) == 0 {
		return "", nil, nil
	}
	sort.Strings(secretNames)

	h := sha256.New()
	for _, name := range secretNames {
		secret := &corev1.Secret{}
		if getErr := r.Get(ctx, types.NamespacedName{Name: name, Namespace: instance.Namespace}, secret); getErr != nil {
			if apierrors.IsNotFound(getErr) {
				h.Write([]byte(name + "=NOT_FOUND\n"))
				missingSecrets = append(missingSecrets, name)
				continue
			}
			return "", nil, fmt.Errorf("failed to get secret %s: %w", name, getErr)
		}
		// Sort keys for determinism
		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k + "="))
			h.Write(secret.Data[k])
			h.Write([]byte("\n"))
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:8]), missingSecrets, nil
}

// findInstancesForSecret maps a Secret change to reconcile requests for
// OpenClawInstances that reference it via envFrom[].secretRef.
func (r *OpenClawInstanceReconciler) findInstancesForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}

	instanceList := &openclawv1alpha1.OpenClawInstanceList{}
	if err := r.List(ctx, instanceList, client.InNamespace(secret.Namespace)); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list OpenClawInstances for secret watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range instanceList.Items {
		instance := &instanceList.Items[i]
		matched := false
		for _, ef := range instance.Spec.EnvFrom {
			if ef.SecretRef != nil && ef.SecretRef.Name == secret.Name {
				matched = true
				break
			}
		}
		if !matched && instance.Spec.Gateway.ExistingSecret == secret.Name {
			matched = true
		}
		if !matched && instance.Spec.Tailscale.Enabled &&
			instance.Spec.Tailscale.AuthKeySecretRef != nil &&
			instance.Spec.Tailscale.AuthKeySecretRef.Name == secret.Name {
			matched = true
		}
		if matched {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager
func (r *OpenClawInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openclawv1alpha1.OpenClawInstance{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.Deployment{}). // temporary: watch legacy Deployments during migration
		Owns(&batchv1.Job{}).       // backup/restore Jobs
		Owns(&batchv1.CronJob{}).   // periodic backup CronJobs
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.findInstancesForSecret)).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.findInstancesForConfigMap)).
		Watches(&openclawv1alpha1.OpenClawClusterDefaults{}, handler.EnqueueRequestsFromMapFunc(r.findInstancesForClusterDefaults)).
		Complete(r)
}

// findInstancesForClusterDefaults enqueues every OpenClawInstance in the
// cluster when the cluster-defaults singleton changes. Only the singleton
// whose name matches ClusterDefaultsSingletonName triggers re-reconciliation;
// other CRs are ignored so typos don't silently churn every instance.
func (r *OpenClawInstanceReconciler) findInstancesForClusterDefaults(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetName() != openclawv1alpha1.ClusterDefaultsSingletonName {
		return nil
	}

	instanceList := &openclawv1alpha1.OpenClawInstanceList{}
	if err := r.List(ctx, instanceList); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list OpenClawInstances for OpenClawClusterDefaults watch")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(instanceList.Items))
	for i := range instanceList.Items {
		instance := &instanceList.Items[i]
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		})
	}
	return requests
}

// findInstancesForConfigMap maps an external ConfigMap change to the OpenClawInstances
// that reference it via spec.config.configMapRef or spec.workspace.configMapRef.
func (r *OpenClawInstanceReconciler) findInstancesForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil
	}

	instanceList := &openclawv1alpha1.OpenClawInstanceList{}
	if err := r.List(ctx, instanceList, client.InNamespace(cm.Namespace)); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list OpenClawInstances for ConfigMap watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range instanceList.Items {
		instance := &instanceList.Items[i]
		matched := false
		// Check spec.config.configMapRef
		if instance.Spec.Config.ConfigMapRef != nil && instance.Spec.Config.ConfigMapRef.Name == cm.Name {
			matched = true
		}
		// Check spec.workspace.configMapRef
		if !matched && instance.Spec.Workspace != nil &&
			instance.Spec.Workspace.ConfigMapRef != nil &&
			instance.Spec.Workspace.ConfigMapRef.Name == cm.Name {
			matched = true
		}
		// Check spec.workspace.additionalWorkspaces[].configMapRef
		if !matched && instance.Spec.Workspace != nil {
			for j := range instance.Spec.Workspace.AdditionalWorkspaces {
				if instance.Spec.Workspace.AdditionalWorkspaces[j].ConfigMapRef != nil &&
					instance.Spec.Workspace.AdditionalWorkspaces[j].ConfigMapRef.Name == cm.Name {
					matched = true
					break
				}
			}
		}
		if matched {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				},
			})
		}
	}
	return requests
}
