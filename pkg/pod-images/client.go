package pod_images

import (
	"context"
	"errors"
	"fmt"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"log"
	"math/rand"
	"strings"
	"time"
)

const maxRetries = 10

type ConnectionConfig struct {
	InClusterConfig        bool
	InformerResyncInterval time.Duration
	ImageCheckInterval     time.Duration
}

type PodImages struct {
	Namespace        string
	Name             string
	Images           map[string]string
	Annotations      map[string]string
	ImagePullSecrets []string
}

type Callback func(images PodImages, removed bool) error

type PodClient struct {
	config    ConnectionConfig
	clientset *kubernetes.Clientset
	factory   informers.SharedInformerFactory
	informer  cache.SharedIndexInformer
	workqueue workqueue.RateLimitingInterface
}

func NewPodClient(config ConnectionConfig) (*PodClient, error) {
	var k8sConfig *rest.Config
	if config.InClusterConfig {
		var err error
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		pathOptions := clientcmd.NewDefaultPathOptions()
		k8sConfig, err = clientcmd.BuildConfigFromKubeconfigGetter("", pathOptions.GetStartingConfig)
		if err != nil {
			return nil, err
		}
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, err
	}

	factory := informers.NewSharedInformerFactory(clientset, config.InformerResyncInterval)
	informer := factory.Core().V1().Pods().Informer()

	queue := workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(time.Second, time.Minute))
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.AddAfter(key, time.Duration(rand.Int63n(time.Minute.Nanoseconds())))
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	})

	return &PodClient{
		config:    config,
		clientset: clientset,
		factory:   factory,
		informer:  informer,
		workqueue: queue,
	}, nil
}

func (p *PodClient) Listen(ctx context.Context, cb Callback) error {
	go p.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), p.informer.HasSynced) {
		return errors.New("failed to sync informer cache")
	}

	for p.processQueueElement(cb) {
	}
	return nil
}

func (p *PodClient) processQueueElement(cb Callback) bool {
	key, quit := p.workqueue.Get()
	if quit {
		return false
	}

	defer p.workqueue.Done(key)

	queueAgain, err := p.processItem(key.(string), cb)

	if err == nil {
		p.workqueue.Forget(key)
		if queueAgain {
			delay := time.Duration(p.config.ImageCheckInterval.Nanoseconds() + rand.Int63n(p.config.ImageCheckInterval.Nanoseconds()/2))
			log.Printf("enqueuing with delay %v", delay)
			p.workqueue.AddAfter(key, delay)
		}
	} else if p.workqueue.NumRequeues(key) < maxRetries {
		log.Printf("error processing %q, will retry", key)
		p.workqueue.AddRateLimited(key)
	} else {
		log.Printf("error processing %q, out of retries", key)
		p.workqueue.Forget(key)
	}

	return true
}

func (p *PodClient) processItem(key string, cb Callback) (bool, error) {
	log.Printf("checking pod %q", key)
	obj, exists, err := p.informer.GetIndexer().GetByKey(key)
	if err != nil {
		return true, err
	}

	if !exists {
		log.Printf("pod deleted %s", key)
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			return false, fmt.Errorf("unexpected key format: %s", key)
		}
		err := cb(PodImages{
			Namespace: parts[0],
			Name:      parts[1],
		}, true)
		return false, err
	}

	pod, ok := obj.(*coreV1.Pod)
	if !ok {
		return true, fmt.Errorf("object is not of type v1/pod: %v", obj)
	}

	images := map[string]string{}
	var imagePullSecrets []string
	for _, imagePullSecret := range pod.Spec.ImagePullSecrets {
		secret, err := p.clientset.CoreV1().Secrets(pod.GetNamespace()).Get(context.Background(), imagePullSecret.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		var decoded []byte
		switch secret.Type {
		case coreV1.SecretTypeDockercfg:
			decoded, ok = secret.Data[coreV1.DockerConfigKey]
		case coreV1.SecretTypeDockerConfigJson:
			decoded, ok = secret.Data[coreV1.DockerConfigJsonKey]
		default:
			log.Printf("unknown secret type %s for secret %s%S", secret.Type, secret.GetNamespace(), secret.GetName())
			continue
		}
		imagePullSecrets = append(imagePullSecrets, string(decoded))
	}

	for _, container := range pod.Spec.Containers {
		images[container.Name] = container.Image
	}

	err = cb(PodImages{
		Namespace:        pod.GetNamespace(),
		Name:             pod.GetName(),
		Images:           images,
		Annotations:      pod.GetAnnotations(),
		ImagePullSecrets: imagePullSecrets,
	}, false)
	return true, err
}
