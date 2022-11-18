/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cappv1alpha1 "github.com/christianh814/welcome-php-operator/api/v1alpha1"
)

// CHX - Setup default image for the WelcomePhp CR
var WelcomePhpImage = "quay.io/redhatworkshops/welcome-php:latest"

// CHX - setup the finalizer
const welcomephpFinalizer = "capp.chernand.io/finalizer"

// CHX - Definitions to manage status conditions
const (
	typeAvailable = "Available"
	typeDegraded  = "Degraded"
)

// WelcomePhpReconciler reconciles a WelcomePhp object
type WelcomePhpReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=capp.chernand.io,resources=welcomephps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=capp.chernand.io,resources=welcomephps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=capp.chernand.io,resources=welcomephps/finalizers,verbs=update
//CHX -add additional RBAC
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the WelcomePhp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *WelcomePhpReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// CHX - begin code

	// get a logger
	log := log.FromContext(ctx)

	// Fetch the WelcomePhp instance - check if the CR for the Kind WelmomePhp exists. If not, return nil to stop
	// the reconciliation process
	welcomephp := &cappv1alpha1.WelcomePhp{}
	err := r.Get(ctx, req.NamespacedName, welcomephp)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// If it's not found, it means it was delted or not yet there. Either way, we don't need to requeue
			log.Info("WelcomePhp resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.Error(err, " - Failed to get WelcomePhp")
		return ctrl.Result{}, err
	}

	// Get the Image
	if len(welcomephp.Spec.ContainerImage) > 0 {
		WelcomePhpImage = welcomephp.Spec.ContainerImage
	}

	// Let's just set the status as Unknown when no status are available
	if welcomephp.Status.Conditions == nil || len(welcomephp.Status.Conditions) == 0 {
		meta.SetStatusCondition(&welcomephp.Status.Conditions, metav1.Condition{
			Type:    typeAvailable,
			Status:  metav1.ConditionUnknown,
			Reason:  "Reconciling",
			Message: "Starting to reconcile",
		})
		if err = r.Status().Update(ctx, welcomephp); err != nil {
			log.Error(err, " - Failed to update WelcomePhp status")
			return ctrl.Result{}, err
		}

		// Let's refetch the resource so that we have the latest state
		if err := r.Get(ctx, req.NamespacedName, welcomephp); err != nil {
			log.Error(err, " - Failed to re-fetch WelcomePhp")
			return ctrl.Result{}, err
		}
	}

	// Let's add a finalizer to the resource. Then we can define some operations to be done when the resource is deleted
	if !controllerutil.ContainsFinalizer(welcomephp, welcomephpFinalizer) {
		log.Info("Adding Finalizer for the WelcomePhp")
		if ok := controllerutil.AddFinalizer(welcomephp, welcomephpFinalizer); !ok {
			log.Error(err, " - Failed to add finalizer")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, welcomephp); err != nil {
			log.Error(err, " - Failed to update WelcomePhp with finalizer")
			return ctrl.Result{}, err
		}
	}

	// Check if the welcomephp instance is marked to be deleted which is indicated by the deletion timestamp being set
	isWelcomePhpMarkedToBeDeleted := welcomephp.GetDeletionTimestamp() != nil
	if isWelcomePhpMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(welcomephp, welcomephpFinalizer) {
			log.Info("Performing finilizer operations for WelcomePhp before deletion")

			// Let's add a status of "Downgrade" to define that this resource began the deletion process
			meta.SetStatusCondition(&welcomephp.Status.Conditions, metav1.Condition{
				Type:    typeDegraded,
				Status:  metav1.ConditionUnknown,
				Reason:  "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the CR: %s", welcomephp.Name),
			})

			if err = r.Status().Update(ctx, welcomephp); err != nil {
				log.Error(err, " - Failed to update WelcomePhp status")
				return ctrl.Result{}, err
			}

			// Perform all operations req before remove the finalizer and allow
			// the K8S API to delete the resource.
			// FUNCTION BELOW
			r.doFinalizerOperationsForWelcomePhp(welcomephp)

			// TODO: If the operations added to the doFinalizerOperationsForWelcomePhp method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the welcomephp resource before updating the status
			if err := r.Get(ctx, req.NamespacedName, welcomephp); err != nil {
				log.Error(err, " - Failed to re-fetch WelcomePhp")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&welcomephp.Status.Conditions, metav1.Condition{
				Type:    typeDegraded,
				Status:  metav1.ConditionTrue,
				Reason:  "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for the CR: %s completed successfully", welcomephp.Name),
			})

			if err := r.Status().Update(ctx, welcomephp); err != nil {
				log.Error(err, " - Failed to update WelcomePhp status")
				return ctrl.Result{}, err
			}

			log.Info("Removing finalizer for the WelcomePhp")
			if ok := controllerutil.RemoveFinalizer(welcomephp, welcomephpFinalizer); !ok {
				log.Error(err, " - Failed to remove finalizer")
				return ctrl.Result{Requeue: true}, nil
			}

			if err = r.Update(ctx, welcomephp); err != nil {
				log.Error(err, " - Failed to remove finalizer from WelcomePhp")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: welcomephp.Name, Namespace: welcomephp.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// define a new deployment
		// FUNCTION BELOW
		dep, err := r.deploymentForWelcomePhp(welcomephp)
		if err != nil {
			log.Error(err, " - Failed to create a new Deployment for WelcomePhp")

			// update the status
			meta.SetStatusCondition(&welcomephp.Status.Conditions, metav1.Condition{
				Type:    typeAvailable,
				Status:  metav1.ConditionFalse,
				Reason:  "Reconciling",
				Message: fmt.Sprintf("Failed to create a new Deployment Custom Resource %s: (%s)", welcomephp.Name, err),
			})

			if err := r.Status().Update(ctx, welcomephp); err != nil {
				log.Error(err, " - Failed to update WelcomePhp status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		log.Info(
			"Creating new deployment",
			"Deployment.Namespace", dep.Namespace,
			"Deployment.Name", dep.Name,
		)
		if err := r.Create(ctx, dep); err != nil {
			log.Error(err, " - Failed to create new Deployment",
				"Deployment.Namespace", dep.Namespace,
				"Deployment.Name", dep.Name,
			)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, " - Failed to get Deployment")
		return ctrl.Result{}, err

	}

	// The CRD API is defining that the WelcomePhp type, have a WelcomePhpSpec.Size field
	// to set the quantity of Deployment instances is the desired state on the cluster.
	// Therefore, the following code will ensure the Deployment size is the same as defined
	// via the Size spec of the Custom Resource which we are reconciling.
	size := welcomephp.Spec.Size
	if *found.Spec.Replicas != size {
		// Increment WelcomePhpDeploymentSizeUndesiredCountTotal metric by 1
		//monitoring.WelcomePhpDeploymentSizeUndesiredCountTotal.Inc()
		found.Spec.Replicas = &size
		if err = r.Update(ctx, found); err != nil {
			log.Error(err, "Failed to update Deployment",
				"Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

			// Re-fetch the welcomephp Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, welcomephp); err != nil {
				log.Error(err, "Failed to re-fetch welcomephp")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&welcomephp.Status.Conditions, metav1.Condition{Type: typeAvailable,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", welcomephp.Name, err)})

			if err := r.Status().Update(ctx, welcomephp); err != nil {
				log.Error(err, "Failed to update WelcomePhp status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&welcomephp.Status.Conditions, metav1.Condition{Type: typeAvailable,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", welcomephp.Name, size)})

	if err := r.Status().Update(ctx, welcomephp); err != nil {
		log.Error(err, "Failed to update Welcomephp status")
		return ctrl.Result{}, err
	}

	// CHX - end code

	return ctrl.Result{}, nil
}

// doFinalizerOperationsForWelcomePhp takes a custom resource as an argument and performs all operations required before the finalizer is removed and the resource is deleted
func (r *WelcomePhpReconciler) doFinalizerOperationsForWelcomePhp(wp *cappv1alpha1.WelcomePhp) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// The following will just raise an event
	r.Recorder.Event(
		wp, "Warning", "Deleting",
		fmt.Sprintf("Deleting the CR %s from namespace %s", wp.Name, wp.Namespace),
	)
}

// deploymentForWelcomePhp takes in type *cappv1alpha1.WelcomePhp and  returns a welcomephp Deployment object and an error
func (r *WelcomePhpReconciler) deploymentForWelcomePhp(wp *cappv1alpha1.WelcomePhp) (*appsv1.Deployment, error) {
	// FUNCTION BELOW
	ls := labelsForWelcomePhp(wp.Name)
	replicas := wp.Spec.Size

	// get the orphaned image
	// FUNCTION BELOW
	image, err := imageForWelcomephp()
	if err != nil {
		return nil, err
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wp.Name,
			Namespace: wp.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						// IMPORTANT: seccomProfile was introduced with Kubernetes 1.19
						// If you are looking for to produce solutions to be supported
						// on lower versions you must remove this option.
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "welcome-php",
						ImagePullPolicy: corev1.PullIfNotPresent,
						// Ensure restrictive context for the container
						// More info: https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
						SecurityContext: &corev1.SecurityContext{
							// WARNING: Ensure that the image used defines an UserID in the Dockerfile
							// otherwise the Pod will not run and will fail with "container has runAsNonRoot and image has non-numeric user"".
							// If you want your workloads admitted in namespaces enforced with the restricted mode in OpenShift/OKD vendors
							// then, you MUST ensure that the Dockerfile defines a User ID OR you MUST leave the "RunAsNonRoot" and
							// "RunAsUser" fields empty.
							RunAsNonRoot: &[]bool{true}[0],
							// The  image does not use a non-zero numeric user as the default user.
							// Due to RunAsNonRoot field being set to true, we need to force the user in the
							// container to a non-zero numeric user. We do this using the RunAsUser field.
							// However, if you are looking to provide solution for K8s vendors like OpenShift
							// be aware that you cannot run under its restricted-v2 SCC if you set this value.
							//RunAsUser:                &[]int64{1001}[0],
							AllowPrivilegeEscalation: &[]bool{false}[0],
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
					}},
				},
			},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(wp, dep, r.Scheme); err != nil {
		return nil, err
	}
	return dep, nil

}

// labelsForWelcomePhp takes a name returns the labels for selecting the resources as a map of strings
func labelsForWelcomePhp(name string) map[string]string {
	var imageTag string

	image, err := imageForWelcomephp()
	if err == nil {
		imageTag = strings.Split(image, ":")[1]
	}

	return map[string]string{
		"app.kubernetes.io/name":       "WelcomePhp",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "welcomephp-operator",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

// imageForWelcomephp gets the Operand image which is managed by this controller
// environment variable defined in the config/manager/manager.yaml
func imageForWelcomephp() (string, error) {
	var image = WelcomePhpImage
	return image, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *WelcomePhpReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cappv1alpha1.WelcomePhp{}).
		Complete(r)
}
