package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	namespace             = "kube_node_metrics"
	metricNameCpuRequests = "cpu_requests"
	metricNameCpuLimits   = "cpu_limits"
	metricNameMemRequests = "mem_requests"
	metricNameMemLimits   = "mem_limits"

	lastFullSyncOkTimeSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "last_full_sync_ok_time_seconds",
			Help:      "Last full sync timestamp in seconds",
		},
	)
	fullSyncDurationSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "full_sync_duration_seconds",
			Help:      "Duration of the full sync",
		},
	)

	k8sApiLatencySeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "k8s_api_latency_seconds",
			Help:      "Latency of the k8s api",
		},
	)

	k8sEventsHandledTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "k8s_events_handled_total",
			Help:      "Total number of k8s events handled",
		},
		[]string{"event_type"},
	)
)

type nodeMeta struct {
	ip   string
	name string

	labels map[string]string
}

type nodeResources struct {
	cpuRequests float64
	cpuLimits   float64
	memRequests float64
	memLimits   float64

	meta *nodeMeta
}

type nodeResourceCollector struct {
	name2nodeResources map[string]*nodeResources

	rwMut sync.RWMutex
}

func (n *nodeResourceCollector) lockedUpdate(name2nodeResources map[string]*nodeResources) {
	n.rwMut.Lock()
	defer n.rwMut.Unlock()

	n.name2nodeResources = name2nodeResources
}

// Collect implements prometheus.Collector.
func (n *nodeResourceCollector) Collect(ch chan<- prometheus.Metric) {
	n.rwMut.RLock()
	defer n.rwMut.RUnlock()

	for _, node := range n.name2nodeResources {
		node.collect(ch)
	}
}

func (nr *nodeResources) collect(ch chan<- prometheus.Metric) {
	labels := nr.meta.labels

	cpuRequest := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Name:        metricNameCpuRequests,
		Help:        "Total CPU requests of all pods running on the node",
		ConstLabels: labels,
	})
	cpuLimit := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Name:        metricNameCpuLimits,
		Help:        "Total CPU limits of all pods running on the node",
		ConstLabels: labels,
	})
	memRequest := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Name:        metricNameMemRequests,
		Help:        "Total memory requests of all pods running on the node",
		ConstLabels: labels,
	})
	memLimit := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Name:        metricNameMemLimits,
		Help:        "Total memory limits of all pods running on the node",
		ConstLabels: labels,
	})

	cpuRequest.Set(nr.cpuRequests)
	cpuLimit.Set(nr.cpuLimits)
	memRequest.Set(nr.memRequests)
	memLimit.Set(nr.memLimits)

	ch <- cpuRequest
	ch <- cpuLimit
	ch <- memRequest
	ch <- memLimit
}

// Describe implements prometheus.Collector.
func (n *nodeResourceCollector) Describe(chan<- *prometheus.Desc) {
}

var _ prometheus.Collector = &nodeResourceCollector{}

var (
	fullSyncInterval = 2 * time.Minute

	k8sApiTimeoutSeconds = int64(30)
	k8sApiPageSize       = int64(1000)
)

func init() {
	prometheus.MustRegister(lastFullSyncOkTimeSeconds)
	prometheus.MustRegister(fullSyncDurationSeconds)
	prometheus.MustRegister(k8sApiLatencySeconds)
	prometheus.MustRegister(k8sEventsHandledTotal)
}

func main() {
	// TODO: support external config
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Error getting cluster config: %v", err)
	}

	config.Timeout = time.Duration(k8sApiTimeoutSeconds) * time.Second
	config.QPS = 100
	config.Burst = 200

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating clientset: %v", err)
	}

	if interval, err := time.ParseDuration(os.Getenv("FULL_SYNC_INTERVAL")); err == nil {
		fullSyncInterval = interval
	}

	log.Printf("Full sync interval: %v", fullSyncInterval)

	collector := newNodeResourceCollector()

	go func() {
		// TODO: set timeout
		ctx := context.Background()

		lastFullSyncOk := time.Now()

		for {
			err := collector.syncFullMetrics(ctx, clientset)
			if err != nil {
				log.Printf("Error collecting metrics: %v", err)
			}
			now := time.Now()
			lastFullSyncOkTimeSeconds.Set(float64(now.Unix()))

			waitingTime := fullSyncInterval - now.Sub(lastFullSyncOk)
			if waitingTime > 0 {
				time.Sleep(waitingTime)
			}

		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":19191", nil))
}

func newNodeResourceCollector() *nodeResourceCollector {
	return &nodeResourceCollector{
		name2nodeResources: make(map[string]*nodeResources),
	}
}

func newNodeMeta(node *v1.Node) *nodeMeta {
	return &nodeMeta{
		ip:   getNodeIp(node),
		name: node.Name,
		labels: map[string]string{
			"node": node.Name,
			"ip":   getNodeIp(node),
		},
	}
}

func getNodeIp(node *v1.Node) string {
	for _, address := range node.Status.Addresses {
		if address.Type == v1.NodeInternalIP {
			return address.Address
		}
	}
	if len(node.Status.Addresses) > 0 {
		return node.Status.Addresses[0].Address
	}
	return "unknown"
}

func getName2NodeResources(nodes []v1.Node) map[string]*nodeResources {
	name2node := make(map[string]*nodeResources)
	for _, node := range nodes {
		name2node[node.Name] = newNodeResource(&node)
	}

	return name2node
}

func newNodeResource(node *v1.Node) *nodeResources {
	meta := newNodeMeta(node)
	return &nodeResources{
		meta: meta,
	}
}

func listNodes(ctx context.Context, clientset *kubernetes.Clientset) ([]v1.Node, error) {
	initialOpts := metav1.ListOptions{
		Limit:          k8sApiPageSize,
		TimeoutSeconds: &k8sApiTimeoutSeconds,
	}

	allNodes := make([]v1.Node, k8sApiPageSize)

	for {
		beg := time.Now()
		nodes, err := clientset.CoreV1().Nodes().List(ctx, initialOpts)
		end := time.Now()
		k8sApiLatencySeconds.Observe(end.Sub(beg).Seconds())

		if err != nil {
			log.Printf("Error listing nodes: %v", err)
			return nil, err
		}
		allNodes = append(allNodes, nodes.Items...)

		if nodes.Continue == "" {
			break
		}
		initialOpts.Continue = nodes.Continue
	}

	return allNodes, nil
}

func listPods(ctx context.Context, clientset *kubernetes.Clientset) ([]v1.Pod, error) {
	nonTerminatedPodsSelector := fields.AndSelectors(
		fields.OneTermNotEqualSelector("status.phase", string(v1.PodSucceeded)),
		fields.OneTermNotEqualSelector("status.phase", string(v1.PodFailed)),
	)

	initialOpts := metav1.ListOptions{
		FieldSelector:  nonTerminatedPodsSelector.String(),
		TimeoutSeconds: &k8sApiTimeoutSeconds,
		Limit:          k8sApiPageSize,
	}

	allPods := make([]v1.Pod, k8sApiPageSize)

	for {
		beg := time.Now()
		pods, err := clientset.CoreV1().Pods("").List(ctx, initialOpts)
		end := time.Now()
		k8sApiLatencySeconds.Observe(end.Sub(beg).Seconds())

		if err != nil {
			log.Printf("Error listing pods: %v", err)
			return nil, err
		}
		allPods = append(allPods, pods.Items...)

		if pods.Continue == "" {
			break
		}
		initialOpts.Continue = pods.Continue
	}

	return allPods, nil
}

func (collector *nodeResourceCollector) syncFullMetrics(ctx context.Context, clientset *kubernetes.Clientset) error {
	beg := time.Now()
	defer func(beg time.Time) {
		end := time.Now()
		fullSyncDurationSeconds.Observe(end.Sub(beg).Seconds())
	}(beg)

	nodes, err := listNodes(ctx, clientset)
	if err != nil {
		log.Printf("Error listing nodes: %v", err)
		return err
	}

	name2nodeResources := getName2NodeResources(nodes)

	allPods, err := listPods(ctx, clientset)
	if err != nil {
		log.Printf("Error listing pods: %v", err)
		return err
	}

	updateResourcesByNode(allPods, name2nodeResources)

	collector.lockedUpdate(name2nodeResources)
	return nil
}

func updateResourcesByNode(pods []v1.Pod, n2r map[string]*nodeResources) {
	for _, pod := range pods {
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			continue
		}

		res, exists := n2r[nodeName]
		if !exists {
			continue
		}

		for _, container := range pod.Spec.Containers {
			if pod.Status.Phase != v1.PodRunning {
				continue
			}

			requests := container.Resources.Requests
			limits := container.Resources.Limits

			if cpu, ok := requests[v1.ResourceCPU]; ok {
				res.cpuRequests += cpu.AsApproximateFloat64()
			}
			if mem, ok := requests[v1.ResourceMemory]; ok {
				res.memRequests += mem.AsApproximateFloat64()
			}
			if cpu, ok := limits[v1.ResourceCPU]; ok {
				res.cpuLimits += cpu.AsApproximateFloat64()
			}
			if mem, ok := limits[v1.ResourceMemory]; ok {
				res.memLimits += mem.AsApproximateFloat64()
			}
		}
	}
}
