package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	namespace    = "kube_node_metrics"
	commonLabels = []string{"node", "ip"}

	cpuRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cpu_reqeust",
			Help:      "Total CPU requests of all pods running on the node",
		},
		commonLabels,
	)

	cpuLimits = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cpu_limit",
			Help:      "Total CPU limits of all pods running on the node",
		},
		commonLabels,
	)

	memRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "memory_request_bytes",
			Help:      "Total memory requests of all pods running on the node",
		},
		commonLabels,
	)

	memLimits = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "memory_limit_bytes",
			Help:      "Total memory limits of all pods running on the node",
		},
		commonLabels,
	)

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
}

type nodeMetrics struct {
	cpuRequests prometheus.Gauge
	cpuLimits   prometheus.Gauge
	memRequests prometheus.Gauge
	memLimits   prometheus.Gauge
}

type nodeResources struct {
	cpuRequests float64
	cpuLimits   float64
	memRequests float64
	memLimits   float64

	meta    *nodeMeta
	metrics *nodeMetrics
}

var (
	fullSyncInterval = 2 * time.Minute

	k8sApiTimeoutSeconds = int64(30)
	k8sApiPageSize       = int64(1000)
)

func init() {
	prometheus.MustRegister(cpuRequests)
	prometheus.MustRegister(cpuLimits)
	prometheus.MustRegister(memRequests)
	prometheus.MustRegister(memLimits)
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

	go func() {
		// TODO: set timeout
		ctx := context.Background()

		lastFullSyncOk := time.Now()

		for {
			err := syncFullMetrics(ctx, clientset)
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

	go watchNodes(clientset)

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":19191", nil))
}

func newNodeMeta(node *v1.Node) *nodeMeta {
	return &nodeMeta{
		ip:   getNodeIp(node),
		name: node.Name,
	}
}

func newNodeMetrics(meta *nodeMeta) *nodeMetrics {
	labels := getNodeLabelValues(meta)
	return &nodeMetrics{
		cpuRequests: cpuRequests.WithLabelValues(labels...),
		cpuLimits:   cpuLimits.WithLabelValues(labels...),
		memRequests: memRequests.WithLabelValues(labels...),
		memLimits:   memLimits.WithLabelValues(labels...),
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
		meta:    meta,
		metrics: newNodeMetrics(meta),
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

func syncFullMetrics(ctx context.Context, clientset *kubernetes.Clientset) error {
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
	updateNodeMetrics(name2nodeResources)

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

func updateNodeMetrics(name2node map[string]*nodeResources) {
	for _, node := range name2node {
		node.metrics.cpuRequests.Set(node.cpuRequests)
		node.metrics.cpuLimits.Set(node.cpuLimits)
		node.metrics.memRequests.Set(node.memRequests)
		node.metrics.memLimits.Set(node.memLimits)
	}
}

func watchNodes(clientset *kubernetes.Clientset) {
	watcher, err := clientset.CoreV1().Nodes().Watch(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Error watching nodes: %v", err)
	}

	for event := range watcher.ResultChan() {
		node, ok := event.Object.(*v1.Node)
		if !ok {
			continue
		}

		switch event.Type {
		case watch.Deleted:
			deleteNodeMetrics(node)
		}
	}
}

func deleteNodeMetrics(node *v1.Node) {
	labelValues := getNodeLabelValues(newNodeMeta(node))

	cpuRequests.DeleteLabelValues(labelValues...)
	cpuLimits.DeleteLabelValues(labelValues...)
	memRequests.DeleteLabelValues(labelValues...)
	memLimits.DeleteLabelValues(labelValues...)
}

func getNodeLabelValues(meta *nodeMeta) []string {
	return []string{
		meta.name,
		meta.ip,
	}
}
