package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kvstoreiov1 "github.com/kvstore/operator/api/v1"
)

// KVStoreReconciler reconciles a KVStore object
type KVStoreReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Config   ReconcilerConfig
}

// ReconcilerConfig holds configuration for the reconciler
type ReconcilerConfig struct {
	MaxConcurrentReconciles int
	Namespace              string
	WatchNamespace         string
	SyncPeriod            time.Duration
	Development           bool
}

// KVStorePhase represents the phase of a KVStore cluster
type KVStorePhase string

const (
	KVStorePhasePending     KVStorePhase = "Pending"
	KVStorePhaseCreating    KVStorePhase = "Creating"
	KVStorePhaseRunning     KVStorePhase = "Running"
	KVStorePhaseScaling     KVStorePhase = "Scaling"
	KVStorePhaseUpdating    KVStorePhase = "Updating"
	KVStorePhaseFailed      KVStorePhase = "Failed"
	KVStorePhaseTerminating KVStorePhase = "Terminating"
)

// KVStoreConditionType represents the type of condition
type KVStoreConditionType string

const (
	KVStoreConditionReady       KVStoreConditionType = "Ready"
	KVStoreConditionProgressing KVStoreConditionType = "Progressing"
	KVStoreConditionDegraded    KVStoreConditionType = "Degraded"
	KVStoreConditionAvailable   KVStoreConditionType = "Available"
)

//+kubebuilder:rbac:groups=kvstore.io,resources=kvstores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kvstore.io,resources=kvstores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kvstore.io,resources=kvstores/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reconciles the KVStore custom resource
func (r *KVStoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("kvstore", req.NamespacedName)

	// Fetch the KVStore instance
	kvstore := &kvstoreiov1.KVStore{}
	err := r.Get(ctx, req.NamespacedName, kvstore)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("KVStore resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get KVStore")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if kvstore.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, kvstore)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(kvstore, kvstoreiov1.KVStoreFinalizer) {
		controllerutil.AddFinalizer(kvstore, kvstoreiov1.KVStoreFinalizer)
		return ctrl.Result{}, r.Update(ctx, kvstore)
	}

	// Update status phase
	if kvstore.Status.Phase == "" {
		kvstore.Status.Phase = string(KVStorePhasePending)
		if err := r.Status().Update(ctx, kvstore); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the cluster
	result, err := r.reconcileCluster(ctx, kvstore)
	if err != nil {
		r.Recorder.Event(kvstore, corev1.EventTypeWarning, "ReconcileFailed", err.Error())
		kvstore.Status.Phase = string(KVStorePhaseFailed)
		r.updateCondition(kvstore, KVStoreConditionReady, metav1.ConditionFalse, "ReconcileFailed", err.Error())
		if statusErr := r.Status().Update(ctx, kvstore); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return result, err
	}

	// Update status
	if err := r.updateStatus(ctx, kvstore); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(kvstore, corev1.EventTypeNormal, "ReconcileSuccess", "Successfully reconciled KVStore cluster")
	return result, nil
}

// reconcileCluster reconciles the entire KVStore cluster
func (r *KVStoreReconciler) reconcileCluster(ctx context.Context, kvstore *kvstoreiov1.KVStore) (ctrl.Result, error) {
	log := r.Log.WithValues("kvstore", kvstore.Name, "namespace", kvstore.Namespace)

	// Set phase to creating if pending
	if kvstore.Status.Phase == string(KVStorePhasePending) {
		kvstore.Status.Phase = string(KVStorePhaseCreating)
		if err := r.Status().Update(ctx, kvstore); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile ConfigMap
	if err := r.reconcileConfigMap(ctx, kvstore); err != nil {
		log.Error(err, "Failed to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	// Reconcile Service (headless)
	if err := r.reconcileHeadlessService(ctx, kvstore); err != nil {
		log.Error(err, "Failed to reconcile headless Service")
		return ctrl.Result{}, err
	}

	// Reconcile Service (cluster)
	if err := r.reconcileClusterService(ctx, kvstore); err != nil {
		log.Error(err, "Failed to reconcile cluster Service")
		return ctrl.Result{}, err
	}

	// Reconcile StatefulSet
	if err := r.reconcileStatefulSet(ctx, kvstore); err != nil {
		log.Error(err, "Failed to reconcile StatefulSet")
		return ctrl.Result{}, err
	}

	// Check if we need to scale
	if err := r.reconcileScaling(ctx, kvstore); err != nil {
		log.Error(err, "Failed to reconcile scaling")
		return ctrl.Result{}, err
	}

	// Reconcile monitoring resources
	if kvstore.Spec.Monitoring.Enabled {
		if err := r.reconcileMonitoring(ctx, kvstore); err != nil {
			log.Error(err, "Failed to reconcile monitoring")
			return ctrl.Result{}, err
		}
	}

	// Reconcile backup resources
	if kvstore.Spec.Backup.Enabled {
		if err := r.reconcileBackup(ctx, kvstore); err != nil {
			log.Error(err, "Failed to reconcile backup")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// reconcileConfigMap reconciles the ConfigMap for KVStore
func (r *KVStoreReconciler) reconcileConfigMap(ctx context.Context, kvstore *kvstoreiov1.KVStore) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvstore.Name + "-config",
			Namespace: kvstore.Namespace,
			Labels:    r.getLabels(kvstore),
		},
		Data: r.generateConfigMapData(kvstore),
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(kvstore, configMap, r.Scheme); err != nil {
		return err
	}

	// Create or update ConfigMap
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, existing)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// Update if different
	if !reflect.DeepEqual(existing.Data, configMap.Data) {
		existing.Data = configMap.Data
		return r.Update(ctx, existing)
	}

	return nil
}

// reconcileHeadlessService reconciles the headless Service for KVStore
func (r *KVStoreReconciler) reconcileHeadlessService(ctx context.Context, kvstore *kvstoreiov1.KVStore) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvstore.Name + "-headless",
			Namespace: kvstore.Namespace,
			Labels:    r.getLabels(kvstore),
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:               "None",
			PublishNotReadyAddresses: true,
			Selector:                 r.getSelectorLabels(kvstore),
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 8080, TargetPort: intstr.FromString("http")},
				{Name: "grpc", Port: 8081, TargetPort: intstr.FromString("grpc")},
				{Name: "raft", Port: 8082, TargetPort: intstr.FromString("raft")},
				{Name: "metrics", Port: 9090, TargetPort: intstr.FromString("metrics")},
			},
		},
	}

	if err := controllerutil.SetControllerReference(kvstore, service, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, existing)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// reconcileClusterService reconciles the cluster Service for KVStore
func (r *KVStoreReconciler) reconcileClusterService(ctx context.Context, kvstore *kvstoreiov1.KVStore) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvstore.Name,
			Namespace: kvstore.Namespace,
			Labels:    r.getLabels(kvstore),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceType(kvstore.Spec.Network.Service.Type),
			Selector: r.getSelectorLabels(kvstore),
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 8080, TargetPort: intstr.FromString("http")},
				{Name: "grpc", Port: 8081, TargetPort: intstr.FromString("grpc")},
			},
		},
	}

	// Add annotations if specified
	if kvstore.Spec.Network.Service.Annotations != nil {
		service.Annotations = kvstore.Spec.Network.Service.Annotations
	}

	if err := controllerutil.SetControllerReference(kvstore, service, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, existing)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// reconcileStatefulSet reconciles the StatefulSet for KVStore
func (r *KVStoreReconciler) reconcileStatefulSet(ctx context.Context, kvstore *kvstoreiov1.KVStore) error {
	statefulSet := r.generateStatefulSet(kvstore)

	if err := controllerutil.SetControllerReference(kvstore, statefulSet, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, existing)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	}

	// Check if update is needed
	if r.shouldUpdateStatefulSet(existing, statefulSet) {
		existing.Spec = statefulSet.Spec
		return r.Update(ctx, existing)
	}

	return nil
}

// generateStatefulSet generates a StatefulSet for the KVStore
func (r *KVStoreReconciler) generateStatefulSet(kvstore *kvstoreiov1.KVStore) *appsv1.StatefulSet {
	labels := r.getLabels(kvstore)
	replicas := kvstore.Spec.Cluster.Replicas

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvstore.Name,
			Namespace: kvstore.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: kvstore.Name + "-headless",
			Replicas:    &replicas,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.StatefulSetUpdateStrategyType(kvstore.Spec.Maintenance.UpdateStrategy),
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					MaxUnavailable: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: int32(kvstore.Spec.Maintenance.MaxUnavailable),
					},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: r.getSelectorLabels(kvstore),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "9090",
						"prometheus.io/path":   "/metrics",
					},
				},
				Spec: r.generatePodSpec(kvstore),
			},
			VolumeClaimTemplates: r.generateVolumeClaimTemplates(kvstore),
		},
	}
}

// generatePodSpec generates the Pod specification for KVStore
func (r *KVStoreReconciler) generatePodSpec(kvstore *kvstoreiov1.KVStore) corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: kvstore.Name,
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &[]bool{true}[0],
			RunAsUser:    &[]int64{1001}[0],
			RunAsGroup:   &[]int64{1001}[0],
			FSGroup:      &[]int64{1001}[0],
		},
		InitContainers: r.generateInitContainers(kvstore),
		Containers:     r.generateContainers(kvstore),
		Volumes:        r.generateVolumes(kvstore),
		Affinity:       r.generateAffinity(kvstore),
		Tolerations:    r.generateTolerations(kvstore),
		NodeSelector:   r.generateNodeSelector(kvstore),
		TerminationGracePeriodSeconds: &[]int64{30}[0],
	}
}

// generateInitContainers generates init containers for KVStore
func (r *KVStoreReconciler) generateInitContainers(kvstore *kvstoreiov1.KVStore) []corev1.Container {
	return []corev1.Container{
		{
			Name:  "init-config",
			Image: "busybox:1.35",
			Command: []string{
				"sh", "-c",
				`echo "Initializing configuration..."
				 cp /config-template/* /config/
				 NODE_ID=${HOSTNAME##*-}
				 sed -i "s/NODE_ID_PLACEHOLDER/$NODE_ID/g" /config/config.yaml
				 echo "Configuration initialized for node $NODE_ID"`,
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "config-template", MountPath: "/config-template"},
				{Name: "config", MountPath: "/config"},
			},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &[]bool{false}[0],
				ReadOnlyRootFilesystem:   &[]bool{true}[0],
			},
		},
	}
}

// generateContainers generates containers for KVStore
func (r *KVStoreReconciler) generateContainers(kvstore *kvstoreiov1.KVStore) []corev1.Container {
	containers := []corev1.Container{
		{
			Name:  "kvstore",
			Image: kvstore.Spec.Cluster.Image,
			Ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: 8080},
				{Name: "grpc", ContainerPort: 8081},
				{Name: "raft", ContainerPort: 8082},
				{Name: "metrics", ContainerPort: 9090},
			},
			Env: r.generateEnvironmentVariables(kvstore),
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(kvstore.Spec.Resources.Requests.CPU),
					corev1.ResourceMemory: resource.MustParse(kvstore.Spec.Resources.Requests.Memory),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(kvstore.Spec.Resources.Limits.CPU),
					corev1.ResourceMemory: resource.MustParse(kvstore.Spec.Resources.Limits.Memory),
				},
			},
			VolumeMounts: r.generateVolumeMounts(kvstore),
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path:   "/health",
						Port:   intstr.FromString("http"),
						Scheme: corev1.URISchemeHTTPS,
					},
				},
				InitialDelaySeconds: 30,
				PeriodSeconds:       10,
				TimeoutSeconds:      5,
				FailureThreshold:    3,
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path:   "/ready",
						Port:   intstr.FromString("http"),
						Scheme: corev1.URISchemeHTTPS,
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       5,
				TimeoutSeconds:      3,
				FailureThreshold:    3,
			},
			StartupProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path:   "/startup",
						Port:   intstr.FromString("http"),
						Scheme: corev1.URISchemeHTTPS,
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       5,
				TimeoutSeconds:      3,
				FailureThreshold:    30,
			},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &[]bool{false}[0],
				ReadOnlyRootFilesystem:   &[]bool{true}[0],
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
			},
		},
	}

	// Add monitoring sidecar if enabled
	if kvstore.Spec.Monitoring.Enabled {
		containers = append(containers, r.generateMonitoringSidecar(kvstore))
	}

	return containers
}

// generateMonitoringSidecar generates a monitoring sidecar container
func (r *KVStoreReconciler) generateMonitoringSidecar(kvstore *kvstoreiov1.KVStore) corev1.Container {
	return corev1.Container{
		Name:  "exporter",
		Image: "kvstore-exporter:latest",
		Ports: []corev1.ContainerPort{
			{Name: "exporter", ContainerPort: 9091},
		},
		Env: []corev1.EnvVar{
			{Name: "KVSTORE_ENDPOINT", Value: "https://localhost:8080"},
			{Name: "EXPORTER_PORT", Value: "9091"},
			{Name: "EXPORTER_LOG_LEVEL", Value: "INFO"},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "tls-certs", MountPath: "/certs", ReadOnly: true},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &[]bool{false}[0],
			ReadOnlyRootFilesystem:   &[]bool{true}[0],
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}
}

// Helper methods for generating various components
func (r *KVStoreReconciler) getLabels(kvstore *kvstoreiov1.KVStore) map[string]string {
	return map[string]string{
		"app":                    "kvstore",
		"kvstore.io/cluster":     kvstore.Name,
		"kvstore.io/version":     kvstore.Spec.Cluster.Version,
		"app.kubernetes.io/name": "kvstore",
		"app.kubernetes.io/instance": kvstore.Name,
		"app.kubernetes.io/version":  kvstore.Spec.Cluster.Version,
		"app.kubernetes.io/component": "database",
		"app.kubernetes.io/part-of":   "kvstore-cluster",
		"app.kubernetes.io/managed-by": "kvstore-operator",
	}
}

func (r *KVStoreReconciler) getSelectorLabels(kvstore *kvstoreiov1.KVStore) map[string]string {
	return map[string]string{
		"app":                "kvstore",
		"kvstore.io/cluster": kvstore.Name,
	}
}

// generateConfigMapData generates configuration data for the ConfigMap
func (r *KVStoreReconciler) generateConfigMapData(kvstore *kvstoreiov1.KVStore) map[string]string {
	return map[string]string{
		"config.yaml": fmt.Sprintf(`
server:
  node_id: NODE_ID_PLACEHOLDER
  cluster_name: %q
  listen_address: "0.0.0.0:8080"
  grpc_address: "0.0.0.0:8081"
  raft_address: "0.0.0.0:8082"
  metrics_address: "0.0.0.0:9090"

storage:
  data_dir: "/data/storage"
  backup_dir: "/data/backup"

raft:
  data_dir: "/data/raft"
  bind_addr: "0.0.0.0:8082"

auth:
  enabled: %t
  jwt:
    secret_file: "/secrets/jwt-secret"

monitoring:
  enabled: %t
  metrics:
    enabled: %t

backup:
  enabled: %t
  interval: %q
`, kvstore.Spec.Cluster.Name,
			kvstore.Spec.Security.Authentication.Enabled,
			kvstore.Spec.Monitoring.Enabled,
			kvstore.Spec.Monitoring.Enabled,
			kvstore.Spec.Backup.Enabled,
			kvstore.Spec.Backup.Schedule),
	}
}

// Continue with the rest of the controller implementation...
// This is a partial implementation showing the structure and key methods

// SetupWithManager sets up the controller with the Manager
func (r *KVStoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kvstoreiov1.KVStore{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Config.MaxConcurrentReconciles,
		}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}