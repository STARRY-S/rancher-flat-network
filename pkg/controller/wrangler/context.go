package wrangler

import (
	"context"
	"sync"

	flscheme "github.com/cnrancher/rancher-flat-network/pkg/generated/clientset/versioned/scheme"
	"github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/apps"
	"github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/batch"
	"github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/core"
	corecontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/core/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/discovery.k8s.io"
	fl "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/flatnetwork.pandaria.io"
	"github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/networking.k8s.io"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/v3/pkg/leader"
	"github.com/rancher/wrangler/v3/pkg/start"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	appsv1 "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/apps/v1"
	batchv1 "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/batch/v1"
	discoveryv1 "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/discovery.k8s.io/v1"
	flv1 "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/flatnetwork.pandaria.io/v1"
	networkingv1 "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/networking.k8s.io/v1"
	ndClientSet "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const controllerName = "rancher-flat-network-operator"

type Context struct {
	RESTConfig        *rest.Config
	Kubernetes        kubernetes.Interface
	ControllerFactory controller.SharedControllerFactory

	FlatNetwork flv1.Interface
	Core        corecontroller.Interface
	Apps        appsv1.Interface
	Networking  networkingv1.Interface
	Batch       batchv1.Interface
	Discovery   discoveryv1.Interface

	// ClientSet for NetworkAttachmentDefinitions
	NDClientSet *ndClientSet.Clientset

	Recorder record.EventRecorder

	supportDiscoveryV1 bool
	supportIngressV1   bool

	leadership *leader.Manager
	starters   []start.Starter

	controllerLock sync.Mutex
}

func NewContextOrDie(
	restCfg *rest.Config,
) *Context {
	// panic on error
	flatnetwork := fl.NewFactoryFromConfigOrDie(restCfg)
	core := core.NewFactoryFromConfigOrDie(restCfg)
	apps := apps.NewFactoryFromConfigOrDie(restCfg)
	networking := networking.NewFactoryFromConfigOrDie(restCfg)
	batch := batch.NewFactoryFromConfigOrDie(restCfg)
	discovery := discovery.NewFactoryFromConfigOrDie(restCfg)
	ndClientSet := ndClientSet.NewForConfigOrDie(restCfg)

	controllerFactory, err := controller.NewSharedControllerFactoryFromConfig(restCfg, runtime.NewScheme())
	if err != nil {
		logrus.Fatalf("failed to build shared controller factory: %v", err)
	}

	utilruntime.Must(flscheme.AddToScheme(scheme.Scheme))

	k8s, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		logrus.Fatalf("kubernetes.NewForConfig: %v", err)
	}
	leadership := leader.NewManager("cattle-flat-network", controllerName, k8s)

	supportDiscoveryV1, err := serverSupportDiscoveryV1(restCfg)
	if err != nil {
		logrus.Fatal(err)
	}
	supportIngressV1 := serverSupportsIngressV1(k8s)

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: k8s.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerName})

	c := &Context{
		RESTConfig:        restCfg,
		Kubernetes:        k8s,
		ControllerFactory: controllerFactory,

		FlatNetwork: flatnetwork.Flatnetwork().V1(),
		Core:        core.Core().V1(),
		Apps:        apps.Apps().V1(),
		Networking:  networking.Networking().V1(),
		Batch:       batch.Batch().V1(),
		Discovery:   discovery.Discovery().V1(),
		NDClientSet: ndClientSet,

		Recorder: recorder,

		supportDiscoveryV1: supportDiscoveryV1,
		supportIngressV1:   supportIngressV1,

		leadership: leadership,
	}
	c.starters = append(c.starters,
		flatnetwork, core, apps, networking, batch, discovery)

	return c
}

func (c *Context) SupportDiscoveryV1() bool {
	return c.supportDiscoveryV1
}

func (c *Context) SupportIngressV1() bool {
	return c.supportIngressV1
}

func (c *Context) OnLeader(f func(ctx context.Context) error) {
	c.leadership.OnLeader(f)
}

func (c *Context) WaitForCacheSyncOrDie(ctx context.Context) {
	if err := c.ControllerFactory.SharedCacheFactory().Start(ctx); err != nil {
		logrus.Fatalf("failed to start shared cache factory: %v", err)
	}
	ok := c.ControllerFactory.SharedCacheFactory().WaitForCacheSync(ctx)
	succeed := true
	for k, v := range ok {
		if !v {
			logrus.Errorf("failed to wait for [%v] cache sync", k)
			succeed = false
		}
	}
	if succeed {
		logrus.Infof("informer cache synced")
	}
}

// Run starts the leader-election process and block.
func (c *Context) Run(ctx context.Context) {
	c.controllerLock.Lock()
	c.leadership.Start(ctx)
	c.controllerLock.Unlock()

	logrus.Infof("waiting for pod becomes leader")
}

func (c *Context) StartHandler(ctx context.Context, worker int) error {
	c.controllerLock.Lock()
	defer c.controllerLock.Unlock()

	return start.All(ctx, worker, c.starters...)
}
