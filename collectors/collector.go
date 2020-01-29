package collectors

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	namespace = "gpfs"
)

var (
	collectorState  = make(map[string]*bool)
	factories       = make(map[string]func() Collector)
	execCommand     = exec.Command
	collectDuration = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "exporter", "collector_duration_seconds"),
		"Collector time duration.",
		[]string{"collector"}, nil)
	collectError = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "exporter", "collect_error"),
		"Indicates if error has occurred during collection",
		[]string{"collector"}, nil)
	lastExecution = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "exporter", "last_execution"),
		"Last execution time of ", []string{"collector"}, nil)
)

type GPFSFilesystem struct {
	Name       string
	Mountpoint string
}

type GPFSCollector struct {
	sync.Mutex
	Collectors map[string]Collector
}

type Collector interface {
	// Get new metrics and expose them via prometheus registry.
	Describe(ch chan<- *prometheus.Desc)
	Collect(ch chan<- prometheus.Metric)
}

func registerCollector(collector string, isDefaultEnabled bool, factory func() Collector) {
	var helpDefaultState string
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	} else {
		helpDefaultState = "disabled"
	}
	flagName := fmt.Sprintf("collector.%s", collector)
	flagHelp := fmt.Sprintf("Enable the %s collector (default: %s).", collector, helpDefaultState)
	defaultValue := fmt.Sprintf("%v", isDefaultEnabled)
	flag := kingpin.Flag(flagName, flagHelp).Default(defaultValue).Bool()
	collectorState[collector] = flag
	factories[collector] = factory
}

func NewGPFSCollector() *GPFSCollector {
	collectors := make(map[string]Collector)
	for key, enabled := range collectorState {
		var collector Collector
		if *enabled {
			collector = factories[key]()
			collectors[key] = collector
		}
	}
	return &GPFSCollector{Collectors: collectors}
}

func SliceContains(slice []string, str string) bool {
	for _, s := range slice {
		if str == s {
			return true
		}
	}
	return false
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func mmlsfs() (string, error) {
	cmd := execCommand("sudo", "/usr/lpp/mmfs/bin/mmlsfs", "all", "-Y", "-T")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Error(err)
		return "", err
	}
	return out.String(), nil
}

func parse_mmlsfs(out string) ([]GPFSFilesystem, error) {
	var filesystems []GPFSFilesystem
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		items := strings.Split(line, ":")
		if len(items) < 7 {
			continue
		}
		if items[2] == "HEADER" {
			continue
		}
		var fs GPFSFilesystem
		fs.Name = items[6]
		mountpoint, err := url.QueryUnescape(items[8])
		if err != nil {
			continue
		}
		fs.Mountpoint = mountpoint
		filesystems = append(filesystems, fs)
	}
	return filesystems, nil
}
