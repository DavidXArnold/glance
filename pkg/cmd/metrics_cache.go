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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// defaultMetricsMinInterval is how long live mode reuses metrics before
// fetching them again. metrics-server scrapes kubelets every 15s by default,
// so polling it more often returns the same numbers while costing a full
// list of every pod's metrics.
const defaultMetricsMinInterval = 10 * time.Second

// metricsCache fetches node and pod metrics at most once per interval and
// serves the last result in between. The metrics API cannot be watched.
type metricsCache struct {
	client   metricsclientset.Interface
	interval time.Duration
	now      func() time.Time

	mu    sync.Mutex
	nodes *cachedMetrics[metricsv1beta1.NodeMetricsList]
	pods  map[string]*cachedMetrics[metricsv1beta1.PodMetricsList]
}

type cachedMetrics[T any] struct {
	list    *T
	err     error
	fetched time.Time
}

func newMetricsCache(client metricsclientset.Interface, interval time.Duration) *metricsCache {
	return &metricsCache{
		client:   client,
		interval: interval,
		now:      time.Now,
		pods:     make(map[string]*cachedMetrics[metricsv1beta1.PodMetricsList]),
	}
}

// fresh reports whether an entry fetched at t can still be served.
func (m *metricsCache) fresh(t time.Time) bool {
	return m.now().Sub(t) < m.interval
}

// NodeMetrics returns node metrics, fetching them if the cached copy is stale.
func (m *metricsCache) NodeMetrics(ctx context.Context) (*metricsv1beta1.NodeMetricsList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nodes != nil && m.fresh(m.nodes.fetched) {
		return m.nodes.list, m.nodes.err
	}
	list, err := m.client.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{ResourceVersion: "0"})
	m.nodes = &cachedMetrics[metricsv1beta1.NodeMetricsList]{list: list, err: err, fetched: m.now()}
	return list, err
}

// PodMetrics returns pod metrics for namespace ("" for all namespaces),
// fetching them if the cached copy is stale. A fresh all-namespaces result
// also serves single-namespace requests.
func (m *metricsCache) PodMetrics(ctx context.Context, namespace string) (*metricsv1beta1.PodMetricsList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.pods[namespace]; c != nil && m.fresh(c.fetched) {
		return c.list, c.err
	}
	if all := m.pods[metav1.NamespaceAll]; namespace != metav1.NamespaceAll && all != nil &&
		all.err == nil && m.fresh(all.fetched) {
		return filterPodMetrics(all.list, namespace), nil
	}
	list, err := m.client.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{ResourceVersion: "0"})
	m.pods[namespace] = &cachedMetrics[metricsv1beta1.PodMetricsList]{list: list, err: err, fetched: m.now()}
	return list, err
}

func filterPodMetrics(all *metricsv1beta1.PodMetricsList, namespace string) *metricsv1beta1.PodMetricsList {
	out := &metricsv1beta1.PodMetricsList{}
	for i := range all.Items {
		if all.Items[i].Namespace == namespace {
			out.Items = append(out.Items, all.Items[i])
		}
	}
	return out
}
