package k8s

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"time"

	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

	"github.com/patrick246/k8s-outdated-image-exporter/pkg/clients"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/tags"
)

const maxRetries = 10

var (
	ErrInformerCacheSync = errors.New("failed to synchronize informer cache")
)

type ConnectionConfig struct {
	InClusterConfig        bool
	InformerResyncInterval time.Duration
	ImageCheckInterval     time.Duration
}

type ContainerClient struct {
	Config ConnectionConfig

	clientset *kubernetes.Clientset
	factory   informers.SharedInformerFactory
	informer  cache.SharedIndexInformer
	workqueue workqueue.RateLimitingInterface

	logger *slog.Logger

	containerCache map[string][]string
}

func NewContainerClient(config ConnectionConfig, logger *slog.Logger) (*ContainerClient, error) {
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

	return &ContainerClient{
		Config:         config,
		clientset:      clientset,
		factory:        factory,
		informer:       informer,
		workqueue:      queue,
		logger:         logger,
		containerCache: map[string][]string{},
	}, nil
}

func (c *ContainerClient) Listener(ctx context.Context) (<-chan clients.ContainerImage, error) {
	go c.informer.Run(ctx.Done())

	containerImageChannel := make(chan clients.ContainerImage)

	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		return nil, ErrInformerCacheSync
	}

	go func() {
		for key, quit := c.workqueue.Get(); !quit; key, quit = c.workqueue.Get() {
			containerImages := c.processWorkqueue(key.(string))

			for _, containerImage := range containerImages {
				containerImageChannel <- containerImage
			}
		}
	}()

	return containerImageChannel, nil
}

func (c *ContainerClient) processWorkqueue(key string) []clients.ContainerImage {
	defer c.workqueue.Done(key)

	c.logger.Info("checking pod", "key", key)

	obj, exists, err := c.informer.GetIndexer().GetByKey(key)
	if err != nil {
		if c.workqueue.NumRequeues(key) < maxRetries {
			c.workqueue.AddRateLimited(key)
		} else {
			c.workqueue.Forget(key)
		}
	}

	if !exists {
		containers, ok := c.containerCache[key]
		if !ok {
			return nil
		}

		delete(c.containerCache, key)

		containerImages := make([]clients.ContainerImage, 0, len(containers))

		for _, container := range containers {
			containerImages = append(containerImages, clients.ContainerImage{
				Name:   container,
				Action: clients.ContainerImageRemoved,
			})
		}

		return containerImages
	}

	pod, ok := obj.(*coreV1.Pod)
	if !ok {
		return nil
	}

	images := map[string]string{}

	for _, container := range pod.Spec.Containers {
		images[container.Name] = container.Image
	}

	var imagePullSecrets []*coreV1.Secret
	if podServiceAccountName := pod.Spec.ServiceAccountName; podServiceAccountName != "" {
		podServiceAccount, err := c.clientset.CoreV1().ServiceAccounts(pod.Namespace).Get(context.Background(), pod.Spec.ServiceAccountName, metav1.GetOptions{})
		if err == nil {
			for _, imagePullSecretRef := range podServiceAccount.ImagePullSecrets {
				secret, err := c.clientset.CoreV1().Secrets(pod.Namespace).Get(context.Background(), imagePullSecretRef.Name, metav1.GetOptions{})
				if err != nil {
					c.logger.Warn("error getting ServiceAccount imagePullSecret. trying without secret", "namespace", pod.Namespace, "name", imagePullSecretRef.Name, "error", err)

					continue
				}

				imagePullSecrets = append(imagePullSecrets, secret)
			}
		} else {
			c.logger.Warn("error getting ServiceAccount. trying without secret", "serviceaccount", podServiceAccountName, "namespace", pod.Namespace, "name", pod.Name)
		}
	}

	for _, imagePullSecretRef := range pod.Spec.ImagePullSecrets {
		secret, err := c.clientset.CoreV1().Secrets(pod.Namespace).Get(context.Background(), imagePullSecretRef.Name, metav1.GetOptions{})
		if err != nil {
			c.logger.Warn("error getting Pod imagePullSecret. trying without secret", "namespace", pod.Namespace, "secretname", imagePullSecretRef.Name, "podname", pod.Name, "error", err)

			continue
		}
		imagePullSecrets = append(imagePullSecrets, secret)
	}

	keychain := tags.RegistryCredentialsFromSecrets(imagePullSecrets)

	containerImages := make([]clients.ContainerImage, 0, len(images))

	for name, image := range images {
		containerImages = append(containerImages, clients.ContainerImage{
			Action: clients.ContainerImageAdded,
			Name:   key + "/" + name,
			Metadata: map[string]interface{}{
				"DockerKeychain": keychain,
			},
			Labels:      pod.Labels,
			Annotations: pod.Annotations,
			Image:       image,
		})
	}

	delay := time.Duration(c.Config.ImageCheckInterval.Nanoseconds() + rand.Int63n(c.Config.ImageCheckInterval.Nanoseconds()/2))

	c.workqueue.AddAfter(key, delay)

	return containerImages
}
