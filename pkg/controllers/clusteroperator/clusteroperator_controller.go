package clusteroperator

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	versionutil "k8s.io/apimachinery/pkg/util/version"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-capi-operator/assets"
	"github.com/openshift/cluster-capi-operator/pkg/controllers"
	"github.com/openshift/cluster-capi-operator/pkg/operatorstatus"
)

const metadataFile = "metadata.yaml"
const configMapVersionLabelName = "provider.cluster.x-k8s.io/version"

// ClusterOperatorReconciler reconciles a ClusterOperator object
type ClusterOperatorReconciler struct {
	operatorstatus.ClusterOperatorStatusClient
	Scheme             *runtime.Scheme
	Images             map[string]string
	PlatformType       string
	SupportedPlatforms map[string]bool
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&configv1.ClusterOperator{}, builder.WithPredicates(clusterOperatorPredicates())).
		Watches(
			&source.Kind{Type: &configv1.Infrastructure{}},
			handler.EnqueueRequestsFromMapFunc(toClusterOperator),
			builder.WithPredicates(infrastructurePredicates()),
		).
		Complete(r)
}

// Reconcile will process the cluster-api clusterOperator
func (r *ClusterOperatorReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithName("ClusterOperatorController")

	log.Info("reconciling Cluster API components for technical preview cluster")
	// Get infrastructure object
	infra := &configv1.Infrastructure{}
	if err := r.Get(ctx, client.ObjectKey{Name: controllers.InfrastructureResourceName}, infra); k8serrors.IsNotFound(err) {
		log.Info("infrastructure cluster does not exist. Skipping...")
		if err := r.SetStatusAvailable(ctx); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "unable to retrive Infrastructure object")
		if err := r.SetStatusDegraded(ctx, err); err != nil {
			return ctrl.Result{}, fmt.Errorf("error syncing ClusterOperatorStatus: %v", err)
		}
		return ctrl.Result{}, err
	}

	// Install core CAPI components
	if err := r.installCoreCAPIComponents(ctx); err != nil {
		log.Error(err, "unable to install core CAPI components")
		if err := r.SetStatusDegraded(ctx, err); err != nil {
			return ctrl.Result{}, fmt.Errorf("error syncing ClusterOperatorStatus: %v", err)
		}
		return ctrl.Result{}, err
	}

	// Set platform type
	if infra.Status.PlatformStatus == nil {
		log.Info("no platform status exists in infrastructure object. Skipping...")
		if err := r.SetStatusAvailable(ctx); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	r.PlatformType = strings.ToLower(string(infra.Status.PlatformStatus.Type))

	// Check if platform type is supported
	if _, ok := r.SupportedPlatforms[r.PlatformType]; !ok {
		log.Info("platform type is not supported. Skipping...", "platformType", r.PlatformType)
		if err := r.SetStatusAvailable(ctx); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Install infrastructure CAPI components
	if err := r.installInfrastructureCAPIComponents(ctx); err != nil {
		log.Error(err, "unable to infrastructure core CAPI components")
		if err := r.SetStatusDegraded(ctx, err); err != nil {
			return ctrl.Result{}, fmt.Errorf("error syncing ClusterOperatorStatus: %v", err)
		}
		return ctrl.Result{}, err
	}

	// repo, err := r.configmapRepository(ctx)
	// if err != nil {
	// 	log.Error(err, "unable to setup CAPI components ConfigMap repository")
	// 	if err := r.SetStatusDegraded(ctx, err); err != nil {
	// 		return ctrl.Result{}, fmt.Errorf("error syncing ClusterOperatorStatus: %v", err)
	// 	}
	// 	return ctrl.Result{}, err
	// }

	return ctrl.Result{}, r.SetStatusAvailable(ctx)
}

// installCoreCAPIComponents reads assets from assets/core-capi, create CRs that are consumed by upstream CAPI Operator
func (r *ClusterOperatorReconciler) installCoreCAPIComponents(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("reconciling Core CAPI components")
	objs, err := assets.ReadCoreProviderAssets(r.Scheme)
	if err != nil {
		return fmt.Errorf("unable to read core-capi: %v", err)
	}

	coreProviderCM := objs[assets.CoreProviderConfigMapKey].(*corev1.ConfigMap)
	if err := r.reconcileConfigMap(ctx, coreProviderCM); err != nil {
		return fmt.Errorf("unable to reconcile core provider ConfigMap: %v", err)
	}

	return nil
}

// installInfrastructureCAPIComponents reads assets from assets/providers, create CRs that are consumed by upstream CAPI Operator
func (r *ClusterOperatorReconciler) installInfrastructureCAPIComponents(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("reconciling Infrastructure CAPI components")
	objs, err := assets.ReadInfrastructureProviderAssets(r.Scheme, r.PlatformType)
	if err != nil {
		return fmt.Errorf("unable to read providers: %v", err)
	}

	infraProviderCM := objs[assets.InfrastructureProviderConfigMapKey].(*corev1.ConfigMap)

	if err := r.reconcileConfigMap(ctx, infraProviderCM); err != nil {
		return fmt.Errorf("unable to reconcile infrastructure provider ConfigMap: %v", err)
	}

	return nil
}

// configmapRepository use clusterctl NewMemoryRepository structure to store the manifests
// and metadata from a given configmap.
func (r *ClusterOperatorReconciler) configmapRepository(ctx context.Context) (repository.Repository, error) {
	mr := repository.NewMemoryRepository()
	mr.WithPaths("", "components.yaml")

	cml := &corev1.ConfigMapList{}
	if err := r.List(ctx, cml, client.HasLabels{configMapVersionLabelName}); err != nil {
		return nil, err
	}
	if len(cml.Items) == 0 {
		return nil, fmt.Errorf("no ConfigMaps found with selector key %s", configMapVersionLabelName)
	}

	for _, cm := range cml.Items {
		version := cm.Name
		errMsg := "from the Name"
		if cm.Labels != nil {
			ver, ok := cm.Labels[configMapVersionLabelName]
			if ok {
				version = ver
				errMsg = "from the Label " + configMapVersionLabelName
			}
		}

		if _, err := versionutil.ParseSemantic(version); err != nil {
			return nil, fmt.Errorf("ConfigMap %s/%s has invalid version: %s (%s)", cm.Namespace, cm.Name, version, errMsg)
		}

		metadata, ok := cm.Data["metadata"]
		if !ok {
			return nil, fmt.Errorf("ConfigMap %s/%s has no metadata", cm.Namespace, cm.Name)
		}
		mr.WithFile(version, metadataFile, []byte(metadata))

		components, ok := cm.Data["components"]
		if !ok {
			return nil, fmt.Errorf("ConfigMap %s/%s has no components", cm.Namespace, cm.Name)
		}
		mr.WithFile(version, mr.ComponentsPath(), []byte(components))
	}

	return mr, nil
}

// // fetch fetches the provider components from the repository and processes all yaml manifests.
// func (r *ClusterOperatorReconciler) fetch(ctx context.Context, repo repository.Repository, options repository.ComponentsOptions, version string) (repository.Components, error) {

// 	// Fetch the provider components yaml file from the provided repository Github/ConfigMap.
// 	componentsFile, err := repo.GetFile(version, repo.ComponentsPath())
// 	if err != nil {
// 		// err = fmt.Errorf("failed to read %q from provider's repository %q: %w", p.repo.ComponentsPath(), p.providerConfig.ManifestLabel(), err)
// 		// return reconcile.Result{}, wrapPhaseError(err, operatorv1.ComponentsFetchErrorReason, operatorv1.PreflightCheckCondition)
// 		return nil, err
// 	}

// 	// Generate a set of new objects using the clusterctl library. NewComponents() will do the yaml proccessing,
// 	// like ensure all the provider components are in proper namespace, replcae variables, etc. See the clusterctl
// 	// documentation for more details.
// 	components, err = repository.NewComponents(repository.ComponentsInput{
// 		Provider:     p.providerConfig,
// 		ConfigClient: p.configClient,
// 		Processor:    yamlprocessor.NewSimpleProcessor(),
// 		RawYaml:      componentsFile,
// 		Options:      options,
// 	})
// 	if err != nil {
// 		// return reconcile.Result{}, wrapPhaseError(err, operatorv1.ComponentsFetchErrorReason, operatorv1.PreflightCheckCondition)
// 		panic(err)
// 	}

// 	// ProviderSpec provides fields for customizing the provider deployment options.
// 	// We can use clusterctl library to apply this customizations.
// 	if err := repository.AlterComponents(components, customizeObjectsFn(p.provider)); err != nil {
// 		// return reconcile.Result{}, wrapPhaseError(err, operatorv1.ComponentsFetchErrorReason, operatorv1.PreflightCheckCondition)
// 		return nil, err
// 	}

// 	return components, nil
// }
