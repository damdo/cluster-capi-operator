package cluster

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/cluster-capi-operator/pkg/operatorstatus"
)

type GenericInfraClusterReconciler struct {
	operatorstatus.ClusterOperatorStatusClient
	InfraCluster client.Object
}

func (r *GenericInfraClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.InfraCluster).
		Complete(r)
}

func (r *GenericInfraClusterReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// log := ctrl.LoggerFrom(ctx).WithName("InfraClusterController")

	// infraClusterCopy := r.InfraCluster.DeepCopyObject().(client.Object)
	// if err := r.Client.Get(ctx, req.NamespacedName, infraClusterCopy); err != nil && !errors.IsNotFound(err) {
	// 	return ctrl.Result{}, err
	// }

	// if !infraClusterCopy.GetDeletionTimestamp().IsZero() {
	// 	return ctrl.Result{}, r.SetStatusAvailable(ctx, "")
	// }

	// log.Info("Reconciling infrastructure cluster")

	// infraClusterPatchCopy, ok := infraClusterCopy.DeepCopyObject().(client.Object)
	// if !ok {
	// 	return ctrl.Result{}, fmt.Errorf("unable to convert to client object")
	// }

	// // Set externally managed annotation
	// infraClusterCopy.SetAnnotations(setManagedByAnnotation(infraClusterCopy.GetAnnotations()))

	// patch := client.MergeFrom(infraClusterPatchCopy)
	// isRequired, err := util.IsPatchRequired(r.InfraCluster, patch)
	// if err != nil {
	// 	return ctrl.Result{}, fmt.Errorf("failed to check if patch required: %w", err)
	// }

	// if isRequired {
	// 	if err := r.Client.Patch(ctx, infraClusterCopy, client.MergeFrom(infraClusterPatchCopy)); err != nil {
	// 		return ctrl.Result{}, fmt.Errorf("unable to patch infra cluster: %v", err)
	// 	}
	// }

	// // Set status to ready
	// unstructuredInfraCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(infraClusterCopy)
	// if err != nil {
	// 	return ctrl.Result{}, fmt.Errorf("unable to convert to unstructured: %v", err)
	// }

	// if err := unstructured.SetNestedField(unstructuredInfraCluster, true, "status", "ready"); err != nil {
	// 	return ctrl.Result{}, fmt.Errorf("unable to set status: %w", err)
	// }

	// if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredInfraCluster, infraClusterCopy); err != nil {
	// 	return ctrl.Result{}, fmt.Errorf("unable to convert from unstructured: %v", err)
	// }

	// patch = client.MergeFrom(infraClusterPatchCopy)
	// isRequired, err = util.IsPatchRequired(infraClusterCopy, patch)
	// if err != nil {
	// 	return ctrl.Result{}, fmt.Errorf("failed to check if patch required: %w", err)
	// }

	// if isRequired {
	// 	if err := r.Status().Patch(ctx, infraClusterCopy, client.MergeFrom(infraClusterPatchCopy)); err != nil {
	// 		return ctrl.Result{}, fmt.Errorf("unable to patch cluster status: %w", err)
	// 	}
	// }

	// return ctrl.Result{}, r.SetStatusAvailable(ctx, "")
	return ctrl.Result{}, nil
}

// func setManagedByAnnotation(annotations map[string]string) map[string]string {
// 	if annotations == nil {
// 		annotations = map[string]string{}
// 	}
// 	annotations[clusterv1.ManagedByAnnotation] = ""

// 	return annotations
// }
