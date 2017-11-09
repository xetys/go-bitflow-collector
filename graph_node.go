package collector

import (
	"regexp"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/antongulenko/golib"
)

const (
	ToleratedUpdateFailures = 2
)

var __nodeID = 0

type collectorNode struct {
	collector Collector
	graph     *collectorGraph
	uniqueID  int

	failedUpdates int
	hasFailed     bool

	metrics MetricReaderMap

	preconditions  []*BoolCondition
	postconditions []*BoolCondition

	UpdateFrequency time.Duration
}

func newCollectorNode(collector Collector, graph *collectorGraph) *collectorNode {
	__nodeID++
	return &collectorNode{
		collector: collector,
		graph:     graph,
		uniqueID:  __nodeID,
	}
}

func (node *collectorNode) String() string {
	return node.collector.String()
}

func (node *collectorNode) init() ([]Collector, error) {
	children, err := node.collector.Init()
	if err != nil {
		return nil, err
	}
	node.metrics = node.collector.Metrics()
	if node.metrics == nil {
		// Implement isInitialized: make sure a successful init() leaves a non-nil metrics map.
		node.metrics = make(MetricReaderMap)
	}
	return children, nil
}

func (node *collectorNode) isInitialized() bool {
	return node.metrics != nil
}

func (node *collectorNode) applyMetricFilters(exclude []*regexp.Regexp, include []*regexp.Regexp) {
	filtered := node.getFilteredMetrics(exclude, include)
	for name := range node.metrics {
		if !filtered[name] {
			delete(node.metrics, name)
		}
	}
}

func (node *collectorNode) getFilteredMetrics(exclude []*regexp.Regexp, include []*regexp.Regexp) map[string]bool {
	filtered := make(map[string]bool)
	for metric := range node.metrics {
		excluded := false
		for _, regex := range exclude {
			if excluded = regex.MatchString(metric); excluded {
				break
			}
		}
		if !excluded && len(include) > 0 {
			excluded = true
			for _, regex := range include {
				if excluded = !regex.MatchString(metric); !excluded {
					break
				}
			}
		}
		if !excluded {
			filtered[metric] = true
		}
	}
	return filtered
}

func (node *collectorNode) loopUpdate(wg *sync.WaitGroup, stopper golib.StopChan) {
	for _, dependsCol := range node.collector.Depends() {
		depends := node.graph.resolve(dependsCol)
		cond := NewBoolCondition()
		node.preconditions = append(node.preconditions, cond)
		depends.postconditions = append(depends.postconditions, cond)
	}
	freq := node.UpdateFrequency
	frequencyLimited := freq > 0
	var lastUpdate time.Time

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			for _, cond := range node.preconditions {
				cond.WaitAndUnset()
			}
			if stopper.Stopped() {
				return
			}
			if !node.graph.nodes[node] {
				// Node has been deleted due to failed dependency
				return
			}

			successfulUpdate := true
			if frequencyLimited {
				now := time.Now()
				if now.Sub(lastUpdate) >= freq {
					successfulUpdate = node.update(stopper)
					lastUpdate = now
				}
			} else {
				successfulUpdate = node.update(stopper)
			}

			for _, cond := range node.postconditions {
				cond.Broadcast()
			}
			if !successfulUpdate || stopper.Stopped() {
				return
			}
		}
	}()
}

func (node *collectorNode) update(stopper golib.StopChan) bool {
	err := node.collector.Update()
	if err == MetricsChanged {
		log.Warnln("Metrics of", node, "have changed! Restarting metric collection.")
		stopper.Stop()
		return false
	} else if err != nil {
		log.Warnln("Update of", node, "failed:", err)
		return !node.updateFailed()
	} else {
		node.failedUpdates = 0
		return true
	}
}

func (node *collectorNode) updateFailed() bool {
	node.failedUpdates++
	if node.failedUpdates >= ToleratedUpdateFailures {
		log.Warnln("Collector", node, "exceeded tolerated number of", ToleratedUpdateFailures, "consecutive failures")
		node.failedUpdates = 0
		node.graph.collectorUpdateFailed(node)
		return true
	}
	return false
}
