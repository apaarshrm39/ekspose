package main

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	applisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type controller struct {
	clientset kubernetes.Interface
	depLister applisters.DeploymentLister
	queue     workqueue.RateLimitingInterface
	hasSynced cache.InformerSynced
}

func newController(clientset kubernetes.Interface, depInformer appsinformers.DeploymentInformer) *controller {
	c := &controller{
		clientset: clientset,
		depLister: depInformer.Lister(),
		queue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "queue"),
		hasSynced: depInformer.Informer().HasSynced,
	}

	depInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleAdd,
			DeleteFunc: c.handleDelete,
		},
	)

	return c
}

func (c *controller) run(ch <-chan struct{}) {
	fmt.Println("starting controller")
	if !cache.WaitForCacheSync(ch, c.hasSynced) {
		fmt.Println("error waiting for cache to be synced")
	}

	go wait.Until(c.worker, 1*time.Second, ch)

	<-ch
}

func (c *controller) worker() {
	for c.processItem() {

	}
}

func (c *controller) processItem() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	// if everything goes fine forget from queue

	defer c.queue.Forget(item)

	key, err := cache.MetaNamespaceKeyFunc(item)
	if err != nil {
		fmt.Println(err)
		return false
	}

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		fmt.Println(err)
		return false
	}

	err = c.syncDeployment(ns, name)
	if err != nil {
		// re-try
		fmt.Println(err)
		return false
	}

	return true
}

func (c *controller) syncDeployment(ns, name string) error {

	// getting deployment from lister
	dep, err := c.depLister.Deployments(ns).Get(name)
	if err != nil {
		fmt.Println(err)
	}
	// create Service
	svc := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Name,
			Namespace: ns,
		},

		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
			},
			Selector: dep.Labels,
		},
	}

	_, err = c.clientset.CoreV1().Services(ns).Create(context.Background(), &svc, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (c *controller) handleAdd(obj interface{}) {
	fmt.Println("add was called")
	c.queue.Add(obj)
}

func (c *controller) handleDelete(obj interface{}) {
	fmt.Println("delete was called")
	c.queue.Add(obj)
}
