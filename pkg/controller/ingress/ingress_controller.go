package ingress

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"

	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new Ingress Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {

	logger := logf.Log.WithName("ingress-controller")
	var c conf
	getConf := c.getConf(os.Getenv("CONFIG_PATH"))
	if getConf == nil {
		logger.Error(fmt.Errorf("Could not find config file %s", os.Getenv("CONFIG_PATH")), fmt.Sprintf("Could not find config file %s", os.Getenv("CONFIG_PATH")))
	}

	return &ReconcileIngress{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		log:    logger,
		config: *getConf,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("ingress-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Ingress
	err = c.Watch(&source.Kind{Type: &extensionsv1beta1.Ingress{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileIngress{}

// ReconcileIngress reconciles a Ingress object
type ReconcileIngress struct {
	client.Client
	scheme *runtime.Scheme
	log    logr.Logger
	config conf
}

// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileIngress) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Ingress instance
	instance := &extensionsv1beta1.Ingress{}

	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.Annotations == nil {
		instance.Annotations = map[string]string{}
	}

	changed := false
	for k, v := range r.config.Annotations.Global {
		if instance.Annotations[k] != v {
			r.log.Info(fmt.Sprintf("Update Ingress: %v annotation: %s", request.Name, k))
			instance.Annotations[k] = v
			changed = true
		}
	}

	for namespace, annotations := range r.config.Annotations.Namespaced {
		if namespace == request.Namespace {
			for k, v := range annotations {
				if instance.Annotations[k] != v {
					r.log.Info(fmt.Sprintf("Update Ingress: %v annotation: %s", request.Name, k))
					instance.Annotations[k] = v
					changed = true
				}
			}
		}
	}

	if changed {
		r.Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

type conf struct {
	Annotations annotationConf `yaml:"annotations"`
}

type annotationConf struct {
	Global     map[string]string            `yaml:"global"`
	Namespaced map[string]map[string]string `yaml:"namespaced"`
}

func (c *conf) getConf(file string) *conf {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		return defaultConf()
	}
	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		return defaultConf()
	}
	return c
}

func defaultConf() *conf {
	return &conf{
		Annotations: annotationConf {
			Global:     map[string]string{},
			Namespaced: map[string]map[string]string{},
		},
	}
}
