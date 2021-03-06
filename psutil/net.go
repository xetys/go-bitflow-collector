package psutil

import (
	"fmt"

	"github.com/bitflow-stream/go-bitflow-collector"
	psnet "github.com/shirou/gopsutil/net"
)

type NetCollector struct {
	collector.AbstractCollector

	factory  *collector.ValueRingFactory
	counters map[string]psnet.IOCountersStat
}

func newNetCollector(root *RootCollector) *NetCollector {
	return &NetCollector{
		AbstractCollector: collector.RootCollector("net-io"),
		factory:           root.Factory,
	}
}

func (col *NetCollector) Init() ([]collector.Collector, error) {
	col.counters = make(map[string]psnet.IOCountersStat)
	if err := col.update(false); err != nil {
		return nil, err
	}

	readers := make([]collector.Collector, 0, len(col.counters)+1)
	for _, nic := range col.counters {
		readers = append(readers, col.newChild(nic.Name, nic.Name))
	}
	readers = append(readers, col.newChild("all", ""))
	return readers, nil
}

func (col *NetCollector) newChild(collectorName string, nicName string) collector.Collector {
	return &psutilNetInterfaceCollector{
		AbstractCollector: col.Child(collectorName),
		parent:            col,
		nicName:           nicName,
		counters:          NewNetIoCounters(col.factory),
	}
}

func (col *NetCollector) MetricsChanged() error {
	return col.Update()
}

func (col *NetCollector) Update() error {
	return col.update(true)
}

func (col *NetCollector) update(checkChange bool) error {
	nicsList, err := psnet.IOCounters(true)
	if err != nil {
		return err
	}
	if checkChange {
		for _, nic := range nicsList {
			if _, ok := col.counters[nic.Name]; !ok {
				return collector.MetricsChanged
			}
		}
		if len(col.counters) != len(nicsList) {
			return collector.MetricsChanged
		}
	}

	nics := make(map[string]psnet.IOCountersStat, len(nicsList))
	for _, nic := range nicsList {
		nics[nic.Name] = nic
	}
	col.counters = nics
	return nil
}

type psutilNetInterfaceCollector struct {
	collector.AbstractCollector
	parent   *NetCollector
	counters NetIoCounters
	nicName  string
}

func (col *psutilNetInterfaceCollector) Depends() []collector.Collector {
	return []collector.Collector{col.parent}
}

func (col *psutilNetInterfaceCollector) Update() error {
	if col.nicName == "" {
		for _, nic := range col.parent.counters {
			col.counters.AddToHead(&nic)
		}
		col.counters.FlushHead()
	} else {
		counters, ok := col.parent.counters[col.nicName]
		if !ok {
			return fmt.Errorf("disk-io counters for disk %v not found", col.nicName)
		}
		col.counters.Add(&counters)
	}
	return nil
}

func (col *psutilNetInterfaceCollector) Metrics() collector.MetricReaderMap {
	prefix := col.nicName
	if prefix == "" {
		prefix = "net-io"
	} else {
		prefix = "net-io/nic/" + prefix
	}
	return col.counters.Metrics(prefix)
}
