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
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

const (
	// BackupSecretName is the name of the Secret containing S3 credentials
	BackupSecretName = "s3-backup-credentials" // #nosec G101 -- not a credential, just a Secret resource name

	// RcloneImage is the rclone container image used for backup jobs.
	RcloneImage = "docker.io/rclone/rclone:1.68"

	// AnnotationSkipBackup allows skipping backup on delete
	AnnotationSkipBackup = "openclaw.rocks/skip-backup"

	// LabelTenant is the label key for the tenant ID
	LabelTenant = "openclaw.rocks/tenant"

	// LabelInstance is the label key for the instance ID
	LabelInstance = "openclaw.rocks/instance"

	// LabelManagedBy is the label key for the manager
	LabelManagedBy = "app.kubernetes.io/managed-by"
)

// s3Credentials holds the S3 credential values read from a Secret
type s3Credentials struct {
	Bucket   string
	KeyID    string
	AppKey   string
	Endpoint string
	Region   string // optional - only needed for S3 providers with custom regions (e.g., MinIO)
	Provider string // rclone S3 provider (e.g., "AWS", "GCS", "Other") - defaults to "Other"
	EnvAuth  bool   // true when static credentials are not provided - uses the provider's credential chain
}

// getTenantID extracts the tenant ID from the instance label or falls back to namespace
func getTenantID(instance *openclawv1alpha1.OpenClawInstance) string {
	if tenant, ok := instance.Labels[LabelTenant]; ok && tenant != "" {
		return tenant
	}
	// Fallback: extract from namespace (oc-tenant-{id} -> {id})
	ns := instance.Namespace
	if strings.HasPrefix(ns, "oc-tenant-") {
		return strings.TrimPrefix(ns, "oc-tenant-")
	}
	return ns
}

// getS3Credentials reads the S3 backup credentials Secret from the operator namespace.
// S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY are optional - when omitted, EnvAuth is set
// to true so rclone uses the AWS SDK credential chain (IRSA / Pod Identity / instance profile).
func (r *OpenClawInstanceReconciler) getS3Credentials(ctx context.Context) (*s3Credentials, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      BackupSecretName,
		Namespace: r.OperatorNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get S3 credentials secret %s/%s: %w", r.OperatorNamespace, BackupSecretName, err)
	}

	get := func(key string) (string, error) {
		v, ok := secret.Data[key]
		if !ok || len(v) == 0 {
			return "", fmt.Errorf("S3 credentials secret missing key %q", key)
		}
		return string(v), nil
	}

	bucket, err := get("S3_BUCKET")
	if err != nil {
		return nil, err
	}
	endpoint, err := get("S3_ENDPOINT")
	if err != nil {
		return nil, err
	}

	// S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY are optional.
	// When both are omitted, EnvAuth is set so rclone uses the provider's credential chain
	// (e.g., AWS SDK chain for IRSA/Pod Identity, GCS Workload Identity).
	keyID := string(secret.Data["S3_ACCESS_KEY_ID"])
	appKey := string(secret.Data["S3_SECRET_ACCESS_KEY"])

	// Validate: either both must be set or both must be empty
	if (keyID == "") != (appKey == "") {
		return nil, fmt.Errorf("S3 credentials secret must set both S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY, or omit both for workload identity (env-auth)")
	}
	envAuth := keyID == "" && appKey == ""

	// S3_REGION is optional - only needed for providers with custom regions (e.g., MinIO)
	region := string(secret.Data["S3_REGION"])

	// S3_PROVIDER sets the rclone --s3-provider flag.
	// Defaults to "Other" for generic S3-compatible backends.
	// Set to "AWS" for native AWS credential chain, "GCS" for Google Cloud Storage
	// S3-compatible endpoint, etc. See: https://rclone.org/s3/#s3-provider
	provider := string(secret.Data["S3_PROVIDER"])
	if provider == "" {
		provider = "Other"
	}

	return &s3Credentials{
		Bucket:   bucket,
		KeyID:    keyID,
		AppKey:   appKey,
		Endpoint: endpoint,
		Region:   region,
		Provider: provider,
		EnvAuth:  envAuth,
	}, nil
}

// mirrorSecretName returns the name of the per-instance mirror Secret that holds
// S3 credentials in the instance namespace (so Jobs can use secretKeyRef).
func mirrorSecretName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name + "-s3-credentials" // #nosec G101 -- not a credential, just a Secret resource name
}

// reconcileS3MirrorSecret creates or updates a mirror of the S3 credentials
// in the instance namespace. This allows Jobs/CronJobs to reference credentials
// via secretKeyRef instead of embedding plaintext values in the Job spec.
// The mirror Secret is owned by the instance and garbage-collected on deletion.
// For env-auth (workload identity) mode, no mirror is needed.
func (r *OpenClawInstanceReconciler) reconcileS3MirrorSecret(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance, creds *s3Credentials) error {
	if creds.EnvAuth {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mirrorSecretName(instance),
			Namespace: instance.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.Labels = map[string]string{
			LabelManagedBy: "openclaw-operator",
			LabelInstance:  instance.Name,
		}
		secret.Data = map[string][]byte{
			"S3_ACCESS_KEY_ID":     []byte(creds.KeyID),
			"S3_SECRET_ACCESS_KEY": []byte(creds.AppKey),
		}
		return controllerutil.SetControllerReference(instance, secret, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile S3 mirror secret: %w", err)
	}
	return nil
}

// buildRcloneJob creates a batch/v1 Job that runs rclone to sync data between a PVC and S3.
// For backup: src=PVC mount, dst=S3 remote path
// For restore: src=S3 remote path, dst=PVC mount
// credentialSecretName is the name of the Secret in the Job's namespace containing
// S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY (used via secretKeyRef to avoid plaintext
// credentials in the Job spec). Ignored when creds.EnvAuth is true.
func buildRcloneJob(
	name, namespace, pvcName string,
	remotePath string,
	labels map[string]string,
	creds *s3Credentials,
	isBackup bool,
	nodeSelector map[string]string,
	tolerations []corev1.Toleration,
	serviceAccountName string,
	credentialSecretName string,
) *batchv1.Job {
	backoffLimit := int32(3)
	ttl := int32(86400) // 24h

	// rclone remote config via env vars
	// :s3: is used because S3-compatible API works with rclone's S3 backend
	rcloneRemotePath := fmt.Sprintf(":s3:%s/%s", creds.Bucket, remotePath)

	var authArgs []string
	if creds.EnvAuth {
		authArgs = []string{"--s3-env-auth=true"}
	} else {
		authArgs = []string{"--s3-access-key-id=$(S3_ACCESS_KEY_ID)", "--s3-secret-access-key=$(S3_SECRET_ACCESS_KEY)"}
	}

	providerFlag := fmt.Sprintf("--s3-provider=%s", creds.Provider)

	var args []string
	if isBackup {
		// PVC -> S3
		args = append([]string{"sync", "/data/", rcloneRemotePath, providerFlag, "--s3-endpoint=$(S3_ENDPOINT)"}, authArgs...)
	} else {
		// S3 -> PVC
		args = append([]string{"sync", rcloneRemotePath, "/data/", providerFlag, "--s3-endpoint=$(S3_ENDPOINT)"}, authArgs...)
	}
	args = append(args, "--skip-links", "--transfers=8", "--checkers=16", "-v")

	if creds.Region != "" {
		args = append(args, "--s3-region=$(S3_REGION)")
	}

	env := []corev1.EnvVar{
		{Name: "S3_ENDPOINT", Value: creds.Endpoint},
	}
	if !creds.EnvAuth {
		env = append(env,
			corev1.EnvVar{
				Name: "S3_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: credentialSecretName},
						Key:                  "S3_ACCESS_KEY_ID",
					},
				},
			},
			corev1.EnvVar{
				Name: "S3_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: credentialSecretName},
						Key:                  "S3_SECRET_ACCESS_KEY",
					},
				},
			},
		)
	}
	if creds.Region != "" {
		env = append(env, corev1.EnvVar{Name: "S3_REGION", Value: creds.Region})
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: serviceAccountName,
					NodeSelector:       nodeSelector,
					Tolerations:        tolerations,
					// Match the fsGroup/runAsUser from the OpenClaw StatefulSet
					// so the rclone container can read/write the PVC data
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  int64Ptr(1000),
						RunAsGroup: int64Ptr(1000),
						FSGroup:    int64Ptr(1000),
					},
					Containers: []corev1.Container{
						{
							Name:    "rclone",
							Image:   RcloneImage,
							Command: []string{"rclone"},
							Args:    args,
							Env:     env,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

// backupJobName returns a deterministic name for the backup Job
func backupJobName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name + "-backup"
}

// restoreJobName returns a deterministic name for the restore Job
func restoreJobName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name + "-restore"
}

// backupLabels returns labels for a backup/restore Job
func backupLabels(instance *openclawv1alpha1.OpenClawInstance, jobType string) map[string]string {
	return map[string]string{
		LabelManagedBy:            "openclaw-operator",
		LabelTenant:               getTenantID(instance),
		LabelInstance:             instance.Name,
		"openclaw.rocks/job-type": jobType,
	}
}

// isJobFinished checks whether the given Job has completed or failed
func isJobFinished(job *batchv1.Job) (bool, batchv1.JobConditionType) {
	for _, c := range job.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
			return true, c.Type
		}
	}
	return false, ""
}

// pvcName returns the PVC name for the instance (delegates to resources package)
func pvcNameForInstance(instance *openclawv1alpha1.OpenClawInstance) string {
	if instance.Spec.Storage.Persistence.ExistingClaim != "" {
		return instance.Spec.Storage.Persistence.ExistingClaim
	}
	return resources.PVCName(instance)
}

// getJob fetches a Job by name and namespace, returns nil if not found
func (r *OpenClawInstanceReconciler) getJob(ctx context.Context, name, namespace string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, job)
	if err != nil {
		return nil, err
	}
	return job, nil
}

// backupCronJobName returns a deterministic name for the periodic backup CronJob
func backupCronJobName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name + "-backup-periodic"
}

// rcloneCronJobEnv returns the environment variables for the rclone CronJob container.
// When creds.EnvAuth is true, static credential env vars are omitted.
// credentialSecretName is the mirror Secret name used via secretKeyRef.
func rcloneCronJobEnv(creds *s3Credentials, credentialSecretName string) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "S3_ENDPOINT", Value: creds.Endpoint},
	}
	if !creds.EnvAuth {
		env = append(env,
			corev1.EnvVar{
				Name: "S3_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: credentialSecretName},
						Key:                  "S3_ACCESS_KEY_ID",
					},
				},
			},
			corev1.EnvVar{
				Name: "S3_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: credentialSecretName},
						Key:                  "S3_SECRET_ACCESS_KEY",
					},
				},
			},
		)
	}
	if creds.Region != "" {
		env = append(env, corev1.EnvVar{Name: "S3_REGION", Value: creds.Region})
	}
	return env
}

// buildBackupCronJob creates a batch/v1 CronJob for periodic S3 backups.
// The CronJob mounts the PVC read-only and uses pod affinity to co-locate
// on the same node as the StatefulSet pod (required for RWO PVCs).
// credentialSecretName is the mirror Secret name used via secretKeyRef
// (ignored when creds.EnvAuth is true).
func buildBackupCronJob(
	instance *openclawv1alpha1.OpenClawInstance,
	creds *s3Credentials,
	credentialSecretName string,
) *batchv1.CronJob {
	name := backupCronJobName(instance)
	labels := backupLabels(instance, "periodic-backup")
	pvcName := pvcNameForInstance(instance)
	tenantID := getTenantID(instance)

	historyLimit := int32(3)
	if instance.Spec.Backup.HistoryLimit != nil {
		historyLimit = *instance.Spec.Backup.HistoryLimit
	}
	failedHistoryLimit := int32(1)
	if instance.Spec.Backup.FailedHistoryLimit != nil {
		failedHistoryLimit = *instance.Spec.Backup.FailedHistoryLimit
	}

	backoffLimit := int32(3)
	ttl := int32(86400)            // 24h
	activeDeadline := int64(3600)  // 1h - kill stuck backup Jobs
	startingDeadline := int64(600) // 10m - skip missed runs rather than firing all at once
	gracePeriod := int64(30)

	// Shell command: incremental sync + daily snapshot + retention cleanup.
	//
	// 1. rclone sync to a fixed "latest" path (incremental - only uploads changed files)
	// 2. rclone copy "latest" to a daily snapshot path (cheap - near-free if today's
	//    snapshot already exists from an earlier run)
	// 3. rclone purge snapshots older than retentionDays
	//
	// This reduces S3 transactions by ~95% vs the old full-re-upload approach and
	// keeps storage bounded by the retention window.
	var authFlags string
	if creds.EnvAuth {
		authFlags = `--s3-env-auth=true`
	} else {
		authFlags = `--s3-access-key-id="${S3_ACCESS_KEY_ID}" ` +
			`--s3-secret-access-key="${S3_SECRET_ACCESS_KEY}"`
	}

	retentionDays := int32(7)
	if instance.Spec.Backup.RetentionDays != nil {
		retentionDays = *instance.Spec.Backup.RetentionDays
	}

	basePath := fmt.Sprintf("backups/%s/%s/periodic", tenantID, instance.Name)
	remote := fmt.Sprintf(":s3:%s/%s", creds.Bucket, basePath)
	s3Flags := fmt.Sprintf(`--s3-provider=%s --s3-endpoint="${S3_ENDPOINT}" %s`, creds.Provider, authFlags)
	if creds.Region != "" {
		s3Flags += ` --s3-region="${S3_REGION}"`
	}

	// CUTOFF uses epoch arithmetic (busybox-compatible, since the rclone image is Alpine-based)
	rcloneCmd := fmt.Sprintf(
		`set -e`+
			` && R="%s"`+
			` && S3="%s"`+
			// Step 1: incremental sync to fixed "latest" path
			` && echo "Step 1: incremental sync to latest"`+
			` && rclone sync /data/ "${S3}/latest" $R --skip-links --transfers=8 --checkers=16 -v`+
			// Step 2: daily snapshot (copy latest to snapshots/YYYY-MM-DD)
			` && TODAY=$(date -u +%%Y-%%m-%%d)`+
			` && echo "Step 2: snapshot ${TODAY}"`+
			` && rclone copy "${S3}/latest" "${S3}/snapshots/${TODAY}" $R --transfers=8 --checkers=16 -v`+
			// Step 3: prune snapshots older than retention period
			` && CUTOFF=$(date -u -d @$(($(date -u +%%s) - 86400 * %d)) +%%Y-%%m-%%d)`+
			` && echo "Step 3: pruning snapshots older than ${CUTOFF} (%d day retention)"`+
			` && for dir in $(rclone lsf "${S3}/snapshots/" $R --dirs-only 2>/dev/null); do`+
			`   d=$(echo "$dir" | tr -d "/");`+
			`   if [ "$d" \< "$CUTOFF" ]; then`+
			`     echo "Pruning snapshot $d";`+
			`     rclone purge "${S3}/snapshots/$d" $R -v;`+
			`   fi;`+
			` done`+
			` && echo "Backup complete"`,
		s3Flags, remote,
		retentionDays, retentionDays,
	)

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   instance.Spec.Backup.Schedule,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			StartingDeadlineSeconds:    &startingDeadline,
			SuccessfulJobsHistoryLimit: &historyLimit,
			FailedJobsHistoryLimit:     &failedHistoryLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: batchv1.JobSpec{
					ActiveDeadlineSeconds:   &activeDeadline,
					BackoffLimit:            &backoffLimit,
					TTLSecondsAfterFinished: &ttl,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: corev1.PodSpec{
							RestartPolicy:                 corev1.RestartPolicyOnFailure,
							DNSPolicy:                     corev1.DNSClusterFirst,
							SchedulerName:                 "default-scheduler",
							TerminationGracePeriodSeconds: &gracePeriod,
							ServiceAccountName:            instance.Spec.Backup.ServiceAccountName,
							NodeSelector:                  instance.Spec.Availability.NodeSelector,
							Tolerations:                   instance.Spec.Availability.Tolerations,
							// Match the StatefulSet pod security context. The PVC must
							// NOT be mounted read-only so that Kubernetes can apply
							// fsGroup ownership (chown to GID 1000) on mount. Without
							// this, the PVC root stays root:root and rclone (UID 1000)
							// gets "permission denied". rclone sync from /data/ is
							// inherently read-only (source path, not destination).
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  int64Ptr(1000),
								RunAsGroup: int64Ptr(1000),
								FSGroup:    int64Ptr(1000),
							},
							// Pod affinity: require scheduling on the same node as the
							// StatefulSet pod so the RWO PVC can be mounted read-only.
							Affinity: &corev1.Affinity{
								PodAffinity: &corev1.PodAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{
													"app.kubernetes.io/name":     "openclaw",
													"app.kubernetes.io/instance": instance.Name,
												},
											},
											TopologyKey: "kubernetes.io/hostname",
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:            "rclone",
									Image:           RcloneImage,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Command:         []string{"sh", "-c", rcloneCmd},
									Env:             rcloneCronJobEnv(creds, credentialSecretName),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "data",
											MountPath: "/data",
										},
									},
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "data",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: pvcName,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// reconcileBackupCronJob creates or deletes the periodic backup CronJob based on spec.backup.schedule.
func (r *OpenClawInstanceReconciler) reconcileBackupCronJob(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	logger := log.FromContext(ctx)

	// If no schedule is set, delete any existing CronJob and clear condition
	if instance.Spec.Backup.Schedule == "" {
		return r.cleanupBackupCronJob(ctx, instance)
	}

	// Check persistence is enabled
	if !resources.IsPersistenceEnabled(instance) {
		logger.Info("Scheduled backup requested but persistence is disabled, skipping CronJob creation")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               openclawv1alpha1.ConditionTypeScheduledBackupReady,
			Status:             metav1.ConditionFalse,
			Reason:             "PersistenceDisabled",
			Message:            "Periodic backups require persistence to be enabled",
			ObservedGeneration: instance.Generation,
		})
		return r.cleanupBackupCronJob(ctx, instance)
	}

	// Get S3 credentials
	creds, err := r.getS3Credentials(ctx)
	if err != nil {
		logger.Info("Scheduled backup requested but S3 credentials not found, skipping CronJob creation", "error", err)
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               openclawv1alpha1.ConditionTypeScheduledBackupReady,
			Status:             metav1.ConditionFalse,
			Reason:             "S3CredentialsMissing",
			Message:            "S3 credentials secret not found in operator namespace - create s3-backup-credentials Secret to enable periodic backups",
			ObservedGeneration: instance.Generation,
		})
		return nil
	}

	// Reconcile mirror Secret for secretKeyRef (no-op for env-auth mode)
	if err := r.reconcileS3MirrorSecret(ctx, instance, creds); err != nil {
		return err
	}

	// Build desired CronJob
	desired := buildBackupCronJob(instance, creds, mirrorSecretName(instance))

	// CreateOrUpdate the CronJob
	obj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupCronJobName(instance),
			Namespace: instance.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		obj.Labels = desired.Labels
		obj.Spec = desired.Spec
		return controllerutil.SetControllerReference(instance, obj, r.Scheme)
	}); err != nil {
		return fmt.Errorf("failed to reconcile backup CronJob: %w", err)
	}

	instance.Status.ManagedResources.BackupCronJob = obj.Name

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               openclawv1alpha1.ConditionTypeScheduledBackupReady,
		Status:             metav1.ConditionTrue,
		Reason:             "CronJobReady",
		Message:            fmt.Sprintf("Periodic backup CronJob %q created with schedule %q", obj.Name, instance.Spec.Backup.Schedule),
		ObservedGeneration: instance.Generation,
	})

	logger.V(1).Info("Backup CronJob reconciled", "name", obj.Name, "schedule", instance.Spec.Backup.Schedule)
	return nil
}

// cleanupBackupCronJob deletes the backup CronJob if it exists and clears status.
func (r *OpenClawInstanceReconciler) cleanupBackupCronJob(ctx context.Context, instance *openclawv1alpha1.OpenClawInstance) error {
	cronJob := &batchv1.CronJob{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      backupCronJobName(instance),
		Namespace: instance.Namespace,
	}, cronJob)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Already gone, clear status
			instance.Status.ManagedResources.BackupCronJob = ""
			meta.RemoveStatusCondition(&instance.Status.Conditions, openclawv1alpha1.ConditionTypeScheduledBackupReady)
			return nil
		}
		return fmt.Errorf("failed to get backup CronJob for cleanup: %w", err)
	}

	if err := r.Delete(ctx, cronJob); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete backup CronJob: %w", err)
	}

	instance.Status.ManagedResources.BackupCronJob = ""
	meta.RemoveStatusCondition(&instance.Status.Conditions, openclawv1alpha1.ConditionTypeScheduledBackupReady)
	return nil
}
