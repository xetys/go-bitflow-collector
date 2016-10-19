package collector

import (
	"sync"
	"time"

	"github.com/antongulenko/data2go"
)

const _max_mock_val = 15

func RegisterMockCollector(factory *ValueRingFactory) {
	RegisterCollector(&MockCollector{
		ring: factory.NewValueRing(),
	})
}

// ==================== Memory ====================
type MockCollector struct {
	AbstractCollector
	val       data2go.Value
	ring      *ValueRing
	startOnce sync.Once
}

func (col *MockCollector) Init() error {
	col.Reset(col)
	col.readers = map[string]MetricReader{
		"mock": col.ring.GetDiff,
	}
	col.startOnce.Do(func() {
		go func() {
			for {
				time.Sleep(333 * time.Millisecond)
				col.val++
				if col.val >= _max_mock_val {
					col.val = 2
				}
			}
		}()
	})
	return nil
}

func (col *MockCollector) Update() error {
	col.ring.Add(StoredValue(col.val))
	col.UpdateMetrics()
	return nil
}