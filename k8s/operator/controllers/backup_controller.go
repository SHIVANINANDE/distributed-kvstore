package controllers

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kvstoreiov1 "github.com/kvstore/operator/api/v1"
)

// BackupController manages backup automation for KVStore clusters
type BackupController struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// BackupPhase represents the phase of a backup operation
type BackupPhase string

const (
	BackupPhasePending   BackupPhase = "Pending"
	BackupPhaseRunning   BackupPhase = "Running"
	BackupPhaseCompleted BackupPhase = "Completed"
	BackupPhaseFailed    BackupPhase = "Failed"
	BackupPhaseVerifying BackupPhase = "Verifying"
)

// BackupType represents the type of backup
type BackupType string

const (
	BackupTypeFull        BackupType = "full"
	BackupTypeIncremental BackupType = "incremental"
	BackupTypeSnapshot    BackupType = "snapshot"
)

// NewBackupController creates a new BackupController
func NewBackupController(client client.Client, log logr.Logger, scheme *runtime.Scheme, recorder record.EventRecorder) *BackupController {
	return &BackupController{
		Client:   client,
		Log:      log,
		Scheme:   scheme,
		Recorder: recorder,
	}
}

// CreateScheduledBackup creates a scheduled backup job for a KVStore cluster
func (b *BackupController) CreateScheduledBackup(ctx context.Context, kvstore *kvstoreiov1.KVStore) error {
	log := b.Log.WithValues("kvstore", kvstore.Name, "namespace", kvstore.Namespace)

	if !kvstore.Spec.Backup.Enabled {
		log.Info("Backup not enabled for cluster")
		return nil
	}

	// Create CronJob for scheduled backups
	cronJob := b.generateBackupCronJob(kvstore)
	if err := controllerutil.SetControllerReference(kvstore, cronJob, b.Scheme); err != nil {
		return err
	}

	existing := &batchv1.CronJob{}
	err := b.Get(ctx, types.NamespacedName{Name: cronJob.Name, Namespace: cronJob.Namespace}, existing)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating backup CronJob", "schedule", kvstore.Spec.Backup.Schedule)
		if err := b.Create(ctx, cronJob); err != nil {
			return fmt.Errorf("failed to create backup CronJob: %w", err)
		}
		b.Recorder.Event(kvstore, corev1.EventTypeNormal, "BackupScheduleCreated", 
			fmt.Sprintf("Created backup schedule: %s", kvstore.Spec.Backup.Schedule))
		return nil
	} else if err != nil {
		return err
	}

	// Update existing CronJob if schedule changed
	if existing.Spec.Schedule != kvstore.Spec.Backup.Schedule {
		existing.Spec = cronJob.Spec
		if err := b.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update backup CronJob: %w", err)
		}
		log.Info("Updated backup CronJob schedule", "newSchedule", kvstore.Spec.Backup.Schedule)
	}

	return nil
}

// ExecuteBackup executes a backup operation
func (b *BackupController) ExecuteBackup(ctx context.Context, backup *kvstoreiov1.KVStoreBackup) error {
	log := b.Log.WithValues("backup", backup.Name, "namespace", backup.Namespace)

	// Update phase to running
	backup.Status.Phase = string(BackupPhaseRunning)
	backup.Status.StartTime = &metav1.Time{Time: time.Now()}
	if err := b.Status().Update(ctx, backup); err != nil {
		return err
	}

	// Get the source KVStore
	kvstore := &kvstoreiov1.KVStore{}
	err := b.Get(ctx, types.NamespacedName{
		Name:      backup.Spec.Source.KVStoreName,
		Namespace: backup.Spec.Source.Namespace,
	}, kvstore)
	if err != nil {
		return fmt.Errorf("failed to get source KVStore: %w", err)
	}

	// Create backup job
	job := b.generateBackupJob(backup, kvstore)
	if err := controllerutil.SetControllerReference(backup, job, b.Scheme); err != nil {
		return err
	}

	if err := b.Create(ctx, job); err != nil {
		return fmt.Errorf("failed to create backup Job: %w", err)
	}

	log.Info("Backup job created", "jobName", job.Name)
	b.Recorder.Event(backup, corev1.EventTypeNormal, "BackupStarted", 
		fmt.Sprintf("Started backup job: %s", job.Name))

	return nil
}

// MonitorBackupProgress monitors the progress of backup operations
func (b *BackupController) MonitorBackupProgress(ctx context.Context, backup *kvstoreiov1.KVStoreBackup) error {
	log := b.Log.WithValues("backup", backup.Name, "namespace", backup.Namespace)

	// Find the backup job
	jobName := b.getBackupJobName(backup)
	job := &batchv1.Job{}
	err := b.Get(ctx, types.NamespacedName{Name: jobName, Namespace: backup.Namespace}, job)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Backup job not found yet")
			return nil
		}
		return err
	}

	// Update backup status based on job status
	if job.Status.Succeeded > 0 {
		backup.Status.Phase = string(BackupPhaseCompleted)
		backup.Status.CompletionTime = job.Status.CompletionTime
		
		// Calculate backup size and location (in real implementation, get from job logs)
		backup.Status.BackupSize = "1.2GB" // Mock value
		backup.Status.Location = b.getBackupLocation(backup)
		
		b.updateBackupCondition(backup, "Completed", metav1.ConditionTrue, "BackupCompleted", "Backup completed successfully")
		b.Recorder.Event(backup, corev1.EventTypeNormal, "BackupCompleted", "Backup completed successfully")
		
	} else if job.Status.Failed > 0 {
		backup.Status.Phase = string(BackupPhaseFailed)
		backup.Status.CompletionTime = &metav1.Time{Time: time.Now()}
		
		b.updateBackupCondition(backup, "Failed", metav1.ConditionTrue, "BackupFailed", "Backup job failed")
		b.Recorder.Event(backup, corev1.EventTypeWarning, "BackupFailed", "Backup job failed")
		
	} else if len(job.Status.Conditions) > 0 {
		// Job is still running
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
				backup.Status.Phase = string(BackupPhaseFailed)
				b.updateBackupCondition(backup, "Failed", metav1.ConditionTrue, "BackupFailed", condition.Message)
				break
			}
		}
	}

	if err := b.Status().Update(ctx, backup); err != nil {
		log.Error(err, "Failed to update backup status")
		return err
	}

	return nil
}

// CleanupOldBackups removes old backups based on retention policy
func (b *BackupController) CleanupOldBackups(ctx context.Context, kvstore *kvstoreiov1.KVStore) error {
	log := b.Log.WithValues("kvstore", kvstore.Name, "namespace", kvstore.Namespace)

	if !kvstore.Spec.Backup.Enabled {
		return nil
	}

	// List all backups for this KVStore
	backupList := &kvstoreiov1.KVStoreBackupList{}
	listOpts := []client.ListOption{
		client.InNamespace(kvstore.Namespace),
		client.MatchingLabels(map[string]string{
			"kvstore.io/cluster": kvstore.Name,
		}),
	}

	if err := b.List(ctx, backupList, listOpts...); err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	// Group backups by type and apply retention policy
	backupsByType := b.groupBackupsByType(backupList.Items)
	
	for backupType, backups := range backupsByType {
		toDelete := b.selectBackupsForDeletion(backups, kvstore.Spec.Backup.Retention, backupType)
		
		for _, backup := range toDelete {
			log.Info("Deleting old backup", "backup", backup.Name, "age", time.Since(backup.CreationTimestamp.Time))
			
			// Delete backup storage first
			if err := b.deleteBackupStorage(ctx, &backup); err != nil {
				log.Error(err, "Failed to delete backup storage", "backup", backup.Name)
				continue
			}
			
			// Delete backup CR
			if err := b.Delete(ctx, &backup); err != nil {
				log.Error(err, "Failed to delete backup CR", "backup", backup.Name)
				continue
			}
			
			b.Recorder.Event(kvstore, corev1.EventTypeNormal, "BackupDeleted", 
				fmt.Sprintf("Deleted old backup: %s", backup.Name))
		}
	}

	return nil
}

// generateBackupCronJob generates a CronJob for scheduled backups
func (b *BackupController) generateBackupCronJob(kvstore *kvstoreiov1.KVStore) *batchv1.CronJob {
	labels := map[string]string{
		"app":                "kvstore-backup",
		"kvstore.io/cluster": kvstore.Name,
		"component":          "backup-scheduler",
	}

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvstore.Name + "-backup-schedule",
			Namespace: kvstore.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          kvstore.Spec.Backup.Schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: b.generateBackupPodSpec(kvstore, "scheduled"),
					},
				},
			},
			SuccessfulJobsHistoryLimit: &[]int32{3}[0],
			FailedJobsHistoryLimit:     &[]int32{3}[0],
		},
	}
}

// generateBackupJob generates a Job for on-demand backups
func (b *BackupController) generateBackupJob(backup *kvstoreiov1.KVStoreBackup, kvstore *kvstoreiov1.KVStore) *batchv1.Job {
	labels := map[string]string{
		"app":                "kvstore-backup",
		"kvstore.io/cluster": kvstore.Name,
		"kvstore.io/backup": backup.Name,
		"component":          "backup-job",
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.getBackupJobName(backup),
			Namespace: backup.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &[]int32{3}[0],
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: b.generateBackupPodSpec(kvstore, string(backup.Spec.Backup.Type)),
			},
		},
	}
}

// generateBackupPodSpec generates the Pod specification for backup jobs
func (b *BackupController) generateBackupPodSpec(kvstore *kvstoreiov1.KVStore, backupType string) corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: kvstore.Name + "-backup",
		RestartPolicy:      corev1.RestartPolicyNever,
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &[]bool{true}[0],
			RunAsUser:    &[]int64{1001}[0],
			RunAsGroup:   &[]int64{1001}[0],
			FSGroup:      &[]int64{1001}[0],
		},
		InitContainers: []corev1.Container{
			{
				Name:  "pre-backup",
				Image: "kvstore-backup:latest",
				Command: []string{
					"sh", "-c",
					`echo "Preparing backup..."
					 mkdir -p /backup/temp
					 echo "Backup preparation complete"`,
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "backup-storage", MountPath: "/backup"},
					{Name: "backup-config", MountPath: "/config"},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &[]bool{false}[0],
					ReadOnlyRootFilesystem:   &[]bool{true}[0],
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
				},
			},
		},
		Containers: []corev1.Container{
			{
				Name:  "backup",
				Image: "kvstore-backup:latest",
				Command: []string{"/usr/local/bin/kvstore-backup"},
				Args: []string{
					"--type=" + backupType,
					"--config=/config/backup.yaml",
					"--source-cluster=" + kvstore.Name,
					"--output-dir=/backup",
					"--compression=" + fmt.Sprintf("%t", kvstore.Spec.Backup.Storage.S3.Encryption != ""),
					"--encryption=" + fmt.Sprintf("%t", kvstore.Spec.Backup.Storage.S3.Encryption != ""),
				},
				Env: []corev1.EnvVar{
					{Name: "BACKUP_TYPE", Value: backupType},
					{Name: "CLUSTER_NAME", Value: kvstore.Name},
					{Name: "NAMESPACE", Value: kvstore.Namespace},
					{Name: "S3_BUCKET", Value: kvstore.Spec.Backup.Storage.S3.Bucket},
					{Name: "S3_REGION", Value: kvstore.Spec.Backup.Storage.S3.Region},
					{Name: "S3_PREFIX", Value: kvstore.Spec.Backup.Storage.S3.Prefix},
					{
						Name: "AWS_ACCESS_KEY_ID",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "kvstore-backup-secrets",
								},
								Key: "s3-access-key",
							},
						},
					},
					{
						Name: "AWS_SECRET_ACCESS_KEY",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "kvstore-backup-secrets",
								},
								Key: "s3-secret-key",
							},
						},
					},
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "backup-storage", MountPath: "/backup"},
					{Name: "backup-config", MountPath: "/config"},
					{Name: "backup-secrets", MountPath: "/secrets"},
					{Name: "source-data", MountPath: "/source", ReadOnly: true},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &[]bool{false}[0],
					ReadOnlyRootFilesystem:   &[]bool{true}[0],
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
				},
			},
			{
				Name:  "verify",
				Image: "kvstore-backup:latest",
				Command: []string{"/usr/local/bin/kvstore-backup-verify"},
				Args: []string{
					"--backup-dir=/backup",
					"--config=/config/backup.yaml",
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "backup-storage", MountPath: "/backup", ReadOnly: true},
					{Name: "backup-config", MountPath: "/config", ReadOnly: true},
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &[]bool{false}[0],
					ReadOnlyRootFilesystem:   &[]bool{true}[0],
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
				},
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "backup-storage",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "kvstore-backup-pvc",
					},
				},
			},
			{
				Name: "backup-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "kvstore-backup-config",
						},
					},
				},
			},
			{
				Name: "backup-secrets",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "kvstore-backup-secrets",
					},
				},
			},
			{
				Name: "source-data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "data-" + kvstore.Name + "-0", // Primary node data
						ReadOnly:  true,
					},
				},
			},
		},
		NodeSelector: map[string]string{
			"node-type": "compute",
		},
		Tolerations: []corev1.Toleration{
			{
				Key:      "node.kubernetes.io/disk-pressure",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
	}
}

// Helper methods

func (b *BackupController) getBackupJobName(backup *kvstoreiov1.KVStoreBackup) string {
	return fmt.Sprintf("%s-backup-job", backup.Name)
}

func (b *BackupController) getBackupLocation(backup *kvstoreiov1.KVStoreBackup) string {
	switch backup.Spec.Storage.Type {
	case "s3":
		return fmt.Sprintf("s3://%s/%s%s.tar.gz", 
			backup.Spec.Storage.S3.Bucket, 
			backup.Spec.Storage.S3.Prefix, 
			backup.Name)
	case "pvc":
		return fmt.Sprintf("/backup/%s.tar.gz", backup.Name)
	default:
		return fmt.Sprintf("/local/backup/%s.tar.gz", backup.Name)
	}
}

func (b *BackupController) updateBackupCondition(backup *kvstoreiov1.KVStoreBackup, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find existing condition or add new one
	found := false
	for i, cond := range backup.Status.Conditions {
		if cond.Type == conditionType {
			backup.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	
	if !found {
		backup.Status.Conditions = append(backup.Status.Conditions, condition)
	}
}

func (b *BackupController) groupBackupsByType(backups []kvstoreiov1.KVStoreBackup) map[BackupType][]kvstoreiov1.KVStoreBackup {
	groups := make(map[BackupType][]kvstoreiov1.KVStoreBackup)
	
	for _, backup := range backups {
		backupType := BackupType(backup.Spec.Backup.Type)
		groups[backupType] = append(groups[backupType], backup)
	}
	
	return groups
}

func (b *BackupController) selectBackupsForDeletion(backups []kvstoreiov1.KVStoreBackup, retention kvstoreiov1.BackupRetention, backupType BackupType) []kvstoreiov1.KVStoreBackup {
	// Sort backups by creation time (newest first)
	sortBackupsByCreationTime(backups)
	
	var toDelete []kvstoreiov1.KVStoreBackup
	now := time.Now()
	
	// Apply retention policy based on backup type
	keepCount := 0
	switch backupType {
	case BackupTypeFull:
		keepCount = retention.Daily
	case BackupTypeIncremental:
		keepCount = retention.Daily * 2 // Keep more incremental backups
	case BackupTypeSnapshot:
		keepCount = retention.Weekly
	default:
		keepCount = retention.Daily
	}
	
	// Keep recent backups and mark old ones for deletion
	for i, backup := range backups {
		age := now.Sub(backup.CreationTimestamp.Time)
		
		// Always keep recent successful backups
		if i < keepCount && backup.Status.Phase == string(BackupPhaseCompleted) {
			continue
		}
		
		// Delete old backups or failed backups older than 24h
		if age > 30*24*time.Hour || 
		   (backup.Status.Phase == string(BackupPhaseFailed) && age > 24*time.Hour) {
			toDelete = append(toDelete, backup)
		}
	}
	
	return toDelete
}

func (b *BackupController) deleteBackupStorage(ctx context.Context, backup *kvstoreiov1.KVStoreBackup) error {
	// In a real implementation, this would delete the backup from S3, GCS, etc.
	// For now, we'll just log the operation
	b.Log.Info("Deleting backup storage", "backup", backup.Name, "location", backup.Status.Location)
	
	// TODO: Implement actual storage deletion based on storage type
	switch backup.Spec.Storage.Type {
	case "s3":
		// Delete from S3
		return b.deleteS3Backup(backup)
	case "gcs":
		// Delete from Google Cloud Storage
		return b.deleteGCSBackup(backup)
	case "azure":
		// Delete from Azure Blob Storage
		return b.deleteAzureBackup(backup)
	case "local", "pvc":
		// Delete from local/PVC storage
		return b.deleteLocalBackup(backup)
	}
	
	return nil
}

// Storage-specific deletion methods (placeholder implementations)

func (b *BackupController) deleteS3Backup(backup *kvstoreiov1.KVStoreBackup) error {
	// TODO: Implement S3 deletion using AWS SDK
	b.Log.Info("Would delete S3 backup", "bucket", backup.Spec.Storage.S3.Bucket, "key", backup.Status.Location)
	return nil
}

func (b *BackupController) deleteGCSBackup(backup *kvstoreiov1.KVStoreBackup) error {
	// TODO: Implement GCS deletion using Google Cloud SDK
	b.Log.Info("Would delete GCS backup", "location", backup.Status.Location)
	return nil
}

func (b *BackupController) deleteAzureBackup(backup *kvstoreiov1.KVStoreBackup) error {
	// TODO: Implement Azure deletion using Azure SDK
	b.Log.Info("Would delete Azure backup", "location", backup.Status.Location)
	return nil
}

func (b *BackupController) deleteLocalBackup(backup *kvstoreiov1.KVStoreBackup) error {
	// TODO: Implement local file deletion
	b.Log.Info("Would delete local backup", "location", backup.Status.Location)
	return nil
}

// Utility function to sort backups by creation time
func sortBackupsByCreationTime(backups []kvstoreiov1.KVStoreBackup) {
	// Simple bubble sort (in production, use sort.Slice)
	n := len(backups)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if backups[j].CreationTimestamp.Before(&backups[j+1].CreationTimestamp) {
				backups[j], backups[j+1] = backups[j+1], backups[j]
			}
		}
	}
}