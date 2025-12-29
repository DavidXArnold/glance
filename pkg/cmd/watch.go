/*
Copyright 2025 David Arnold
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

package cmd

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// WatchCache provides cached cluster data using informers for real-time updates.
type WatchCache struct {
	mu sync.RWMutex

	// Cached data
	nodes      []v1.Node
	pods       []v1.Pod
	namespaces []v1.Namespace

	// Informer factory
	factory informers.SharedInformerFactory
	stopCh  chan struct{}

	// Change notification
	updateCh chan struct{}

	// Stats
	lastUpdate time.Time
	nodeCount  int
	podCount   int
}

// NewWatchCache creates a new informer-based cache for cluster data.
func NewWatchCache(k8sClient *kubernetes.Clientset, resyncPeriod time.Duration) *WatchCache {
	wc := &WatchCache{
		stopCh:   make(chan struct{}),
		updateCh: make(chan struct{}, 1), // Buffered to avoid blocking
	}

	// Create shared informer factory with resync period
	wc.factory = informers.NewSharedInformerFactory(k8sClient, resyncPeriod)

	// Set up node informer
	nodeInformer := wc.factory.Core().V1().Nodes().Informer()
	_, _ = nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { wc.notifyUpdate() },
		UpdateFunc: func(_, _ interface{}) { wc.notifyUpdate() },
		DeleteFunc: func(_ interface{}) { wc.notifyUpdate() },
	})

	// Set up pod informer
	podInformer := wc.factory.Core().V1().Pods().Informer()
	_, _ = podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { wc.notifyUpdate() },
		UpdateFunc: func(_, _ interface{}) { wc.notifyUpdate() },
		DeleteFunc: func(_ interface{}) { wc.notifyUpdate() },
	})

	// Set up namespace informer
	nsInformer := wc.factory.Core().V1().Namespaces().Informer()
	_, _ = nsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { wc.notifyUpdate() },
		UpdateFunc: func(_, _ interface{}) { wc.notifyUpdate() },
		DeleteFunc: func(_ interface{}) { wc.notifyUpdate() },
	})

	return wc
}

// Start begins the informer sync process.
func (wc *WatchCache) Start(ctx context.Context) error {
	// Start the informer factory
	wc.factory.Start(wc.stopCh)

	// Wait for initial cache sync
	log.Debug("Waiting for informer caches to sync...")
	synced := wc.factory.WaitForCacheSync(wc.stopCh)
	for informerType, ok := range synced {
		if !ok {
			log.Warnf("Failed to sync cache for: %v", informerType)
		}
	}
	log.Debug("Informer caches synced")

	// Initial data load
	wc.refreshData()

	return nil
}

// Stop stops the informers.
func (wc *WatchCache) Stop() {
	close(wc.stopCh)
}

// Updates returns a channel that receives notifications when data changes.
func (wc *WatchCache) Updates() <-chan struct{} {
	return wc.updateCh
}

// notifyUpdate sends a non-blocking notification of data change.
func (wc *WatchCache) notifyUpdate() {
	wc.refreshData()
	select {
	case wc.updateCh <- struct{}{}:
	default:
		// Channel full, update already pending
	}
}

// refreshData updates the cached data from informers.
func (wc *WatchCache) refreshData() {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	// Get nodes from cache
	nodeList, err := wc.factory.Core().V1().Nodes().Lister().List(nil)
	if err != nil {
		log.Debugf("Failed to list nodes from cache: %v", err)
		return
	}
	wc.nodes = make([]v1.Node, 0, len(nodeList))
	for _, n := range nodeList {
		wc.nodes = append(wc.nodes, *n)
	}
	wc.nodeCount = len(wc.nodes)

	// Get pods from cache
	podList, err := wc.factory.Core().V1().Pods().Lister().List(nil)
	if err != nil {
		log.Debugf("Failed to list pods from cache: %v", err)
		return
	}
	wc.pods = make([]v1.Pod, 0, len(podList))
	for _, p := range podList {
		wc.pods = append(wc.pods, *p)
	}
	wc.podCount = len(wc.pods)

	// Get namespaces from cache
	nsList, err := wc.factory.Core().V1().Namespaces().Lister().List(nil)
	if err != nil {
		log.Debugf("Failed to list namespaces from cache: %v", err)
		return
	}
	wc.namespaces = make([]v1.Namespace, 0, len(nsList))
	for _, ns := range nsList {
		wc.namespaces = append(wc.namespaces, *ns)
	}

	wc.lastUpdate = time.Now()
}

// GetNodes returns a copy of cached nodes.
func (wc *WatchCache) GetNodes() []v1.Node {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	result := make([]v1.Node, len(wc.nodes))
	copy(result, wc.nodes)
	return result
}

// GetPods returns a copy of cached pods.
func (wc *WatchCache) GetPods() []v1.Pod {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	result := make([]v1.Pod, len(wc.pods))
	copy(result, wc.pods)
	return result
}

// GetPodsByNode returns pods grouped by node name.
func (wc *WatchCache) GetPodsByNode() map[string][]v1.Pod {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	result := make(map[string][]v1.Pod)
	for _, pod := range wc.pods {
		if pod.Spec.NodeName != "" && pod.Status.Phase != v1.PodSucceeded && pod.Status.Phase != v1.PodFailed {
			result[pod.Spec.NodeName] = append(result[pod.Spec.NodeName], pod)
		}
	}
	return result
}

// GetPodsByNamespace returns pods grouped by namespace.
func (wc *WatchCache) GetPodsByNamespace() map[string][]v1.Pod {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	result := make(map[string][]v1.Pod)
	for _, pod := range wc.pods {
		result[pod.Namespace] = append(result[pod.Namespace], pod)
	}
	return result
}

// GetNamespaces returns a copy of cached namespaces.
func (wc *WatchCache) GetNamespaces() []v1.Namespace {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	result := make([]v1.Namespace, len(wc.namespaces))
	copy(result, wc.namespaces)
	return result
}

// GetStats returns cache statistics.
func (wc *WatchCache) GetStats() (nodeCount, podCount int, lastUpdate time.Time) {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	return wc.nodeCount, wc.podCount, wc.lastUpdate
}

// GetNodeByName returns a specific node by name.
func (wc *WatchCache) GetNodeByName(name string) (*v1.Node, bool) {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	for i := range wc.nodes {
		if wc.nodes[i].Name == name {
			node := wc.nodes[i]
			return &node, true
		}
	}
	return nil, false
}

// ListNodesWithSelector returns nodes matching a label selector.
func (wc *WatchCache) ListNodesWithSelector(selector string) ([]v1.Node, error) {
	if selector == "" {
		return wc.GetNodes(), nil
	}

	wc.mu.RLock()
	defer wc.mu.RUnlock()

	labelSelector, err := metav1.ParseToLabelSelector(selector)
	if err != nil {
		return nil, err
	}

	sel, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return nil, err
	}

	var result []v1.Node
	for _, node := range wc.nodes {
		if sel.Matches(labelSetFromMap(node.Labels)) {
			result = append(result, node)
		}
	}
	return result, nil
}

// labelSetFromMap converts a map to labels.Set for selector matching
type labelSet map[string]string

func labelSetFromMap(m map[string]string) labelSet {
	return labelSet(m)
}

func (ls labelSet) Has(key string) bool {
	_, exists := ls[key]
	return exists
}

func (ls labelSet) Get(key string) string {
	return ls[key]
}
