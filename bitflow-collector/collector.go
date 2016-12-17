package main

import (
	"errors"
	"flag"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/antongulenko/go-bitflow-collector"
	"github.com/antongulenko/go-bitflow-collector/libvirt"
	"github.com/antongulenko/go-bitflow-collector/mock"
	"github.com/antongulenko/go-bitflow-collector/ovsdb"
	"github.com/antongulenko/go-bitflow-collector/pcap"
	"github.com/antongulenko/go-bitflow-collector/psutil"
	"github.com/antongulenko/golib"
)

var (
	collect_local_interval = 500 * time.Millisecond
	sink_interval          = 500 * time.Millisecond

	all_metrics           = false
	include_basic_metrics = false
	user_include_metrics  golib.StringSlice
	user_exclude_metrics  golib.StringSlice

	proc_collectors      golib.StringSlice
	proc_collector_regex golib.StringSlice
	proc_show_errors     = false
	proc_update_pids     = 1500 * time.Millisecond

	libvirt_uri = libvirt.LibvirtLocal() // collector.LibvirtSsh("host", "keyfile")
	ovsdb_host  = ""

	pcap_nics = ""

	ringFactory = collector.ValueRingFactory{
		// This is an important package-wide constant: time-window for all aggregated values
		Interval: 1000 * time.Millisecond,
	}
)

var (
	includeMetricsRegexes []*regexp.Regexp
	excludeMetricsRegexes = []*regexp.Regexp{
		regexp.MustCompile("^mock$"),
		regexp.MustCompile("^net-proto/(UdpLite|IcmpMsg)"),                         // Some extended protocol-metrics
		regexp.MustCompile("^disk-io/[^all]"),                                      // Disk IO for specific partitions/disks
		regexp.MustCompile("^disk-usage/[^all]"),                                   // Disk usage for specific partitions
		regexp.MustCompile("^net-proto/tcp/(MaxConn|RtpAlgorithm|RtpMin|RtoMax)$"), // Some irrelevant TCP/IP settings
		regexp.MustCompile("^net-proto/ip/(DefaultTTL|Forwarding)$"),
	}
	includeBasicMetricsRegexes = []*regexp.Regexp{
		regexp.MustCompile("^(cpu|mem/percent)$"),
		regexp.MustCompile("^disk-io/.../(io|ioTime|ioBytes)$"),
		regexp.MustCompile("^net-io/(bytes|packets|dropped|errors)$"),
		regexp.MustCompile("^proc/.+/(cpu|mem/rss|disk/(io|ioBytes)|net-io/(bytes|packets|dropped|errors))$"),
	}
)

func init() {
	flag.StringVar(&libvirt_uri, "libvirt", libvirt_uri, "Libvirt connection uri (default is local system)")
	flag.StringVar(&ovsdb_host, "ovsdb", ovsdb_host, "OVSDB host to connect to. Empty for localhost. Port is "+strconv.Itoa(ovsdb.DefaultOvsdbPort))
	flag.BoolVar(&all_metrics, "a", all_metrics, "Disable built-in filters on available metrics")
	flag.Var(&user_exclude_metrics, "exclude", "Metrics to exclude (only with -c, substring match)")
	flag.Var(&user_include_metrics, "include", "Metrics to include exclusively (only with -c, substring match)")
	flag.BoolVar(&include_basic_metrics, "basic", include_basic_metrics, "Include only a certain basic subset of metrics")

	flag.Var(&proc_collectors, "proc", "'key=substring' Processes to collect metrics for (substring match on entire command line)")
	flag.Var(&proc_collector_regex, "proc-regex", "'key=regex' Processes to collect metrics for (regex match on entire command line)")
	flag.BoolVar(&proc_show_errors, "proc-show-errors", proc_show_errors, "Verbose: show errors encountered while getting process metrics")
	flag.DurationVar(&proc_update_pids, "proc-interval", proc_update_pids, "Interval for updating list of observed pids")

	flag.DurationVar(&collect_local_interval, "ci", collect_local_interval, "Interval for collecting local samples")
	flag.DurationVar(&sink_interval, "si", sink_interval, "Interval for sinking (sending/printing/...) data when collecting local samples")

	flag.StringVar(&pcap_nics, "nics", pcap_nics, "Comma-separated list of NICs to capture packets from for PCAP-based"+
		"monitoring of process network IO (/proc/.../net-pcap/...). Defaults to all physical NICs.")
}

func configurePcap() {
	if pcap_nics == "" {
		allNics, err := pcap.PhysicalInterfaces()
		if err != nil {
			log.Fatalln("Failed to enumerate physical NICs:", err)
		}
		psutil.PcapNics = allNics
	} else {
		psutil.PcapNics = strings.Split(pcap_nics, ",")
	}
}

func createCollectorSource() *collector.CollectorSource {
	ringFactory.Length = int(ringFactory.Interval/collect_local_interval) * 3 // Make sure enough samples can be buffered
	mock.RegisterMockCollector(&ringFactory)
	psutil.RegisterPsutilCollectors(collect_local_interval*3/2, &ringFactory) // Update PIDs less often then metrics
	libvirt.RegisterLibvirtCollector(libvirt_uri, &ringFactory)
	ovsdb.RegisterOvsdbCollector(ovsdb_host, &ringFactory)
	if len(proc_collectors) > 0 || len(proc_collector_regex) > 0 {
		regexes := make(map[string][]*regexp.Regexp)
		for _, substr := range proc_collectors {
			key, value := splitKeyValue(substr)
			regex := regexp.MustCompile(regexp.QuoteMeta(value))
			regexes[key] = append(regexes[key], regex)
		}
		for _, regexStr := range proc_collector_regex {
			key, value := splitKeyValue(regexStr)
			regex, err := regexp.Compile(value)
			golib.Checkerr(err)
			regexes[key] = append(regexes[key], regex)
		}
		for key, list := range regexes {
			collector.RegisterCollector(&psutil.PsutilProcessCollector{
				CmdlineFilter:     list,
				GroupName:         key,
				PrintErrors:       proc_show_errors,
				PidUpdateInterval: proc_update_pids,
				Factory:           &ringFactory,
			})
		}
	}

	if all_metrics {
		excludeMetricsRegexes = nil
	}
	if include_basic_metrics {
		includeMetricsRegexes = append(includeMetricsRegexes, includeBasicMetricsRegexes...)
	}
	for _, exclude := range user_exclude_metrics {
		excludeMetricsRegexes = append(excludeMetricsRegexes,
			regexp.MustCompile(regexp.QuoteMeta(exclude)))
	}
	for _, include := range user_include_metrics {
		includeMetricsRegexes = append(includeMetricsRegexes,
			regexp.MustCompile(regexp.QuoteMeta(include)))
	}

	return &collector.CollectorSource{
		CollectInterval: collect_local_interval,
		SinkInterval:    sink_interval,
		ExcludeMetrics:  excludeMetricsRegexes,
		IncludeMetrics:  includeMetricsRegexes,
	}
}

func splitKeyValue(pair string) (string, string) {
	index := strings.Index(pair, "=")
	if index > 0 {
		return pair[:index], pair[index+1:]
	}
	golib.Checkerr(errors.New("-proc and -proc_regex must have argument format 'key=value'"))
	return "", ""
}
