// +build !nostat

package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// #include <unistd.h>
import "C"

const (
	procStat = "/proc/stat"
)

type statCollector struct {
	config       Config
	cpu          *prometheus.CounterVec
	intr         prometheus.Counter
	ctxt         prometheus.Counter
	forks        prometheus.Counter
	btime        prometheus.Gauge
	procsRunning prometheus.Gauge
	procsBlocked prometheus.Gauge
}

func init() {
	Factories["stat"] = NewStatCollector
}

// Takes a config struct and prometheus registry and returns a new Collector exposing
// network device stats.
func NewStatCollector(config Config) (Collector, error) {
	return &statCollector{
		config: config,
		cpu: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "cpu",
				Help:      "Seconds the cpus spent in each mode.",
			},
			[]string{"cpu", "mode"},
		),
		intr: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "intr",
			Help:      "Total number of interrupts serviced.",
		}),
		ctxt: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "context_switches",
			Help:      "Total number of context switches.",
		}),
		forks: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "forks",
			Help:      "Total number of forks.",
		}),
		btime: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "boot_time",
			Help:      "Node boot time, in unixtime.",
		}),
		procsRunning: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "procs_running",
			Help:      "Number of processes in runnable state.",
		}),
		procsBlocked: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "procs_blocked",
			Help:      "Number of processes blocked waiting for I/O to complete.",
		}),
	}, nil
}

// Expose a variety of stats from /proc/stats.
func (c *statCollector) Update(ch chan<- prometheus.Metric) (err error) {
	file, err := os.Open(procStat)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) == 0 {
			continue
		}
		switch {
		case strings.HasPrefix(parts[0], "cpu"):
			// Export only per-cpu stats, it can be aggregated up in prometheus.
			if parts[0] == "cpu" {
				break
			}
			// Only some of these may be present, depending on kernel version.
			cpuFields := []string{"user", "nice", "system", "idle", "iowait", "irq", "softirq", "steal", "guest"}
			// OpenVZ guests lack the "guest" CPU field, which needs to be ignored.
			expectedFieldNum := len(cpuFields)+1
			if expectedFieldNum > len(parts) {
				expectedFieldNum = len(parts)
			}
			for i, v := range parts[1 : expectedFieldNum] {
				value, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return err
				}
				// Convert from ticks to seconds
				value /= float64(C.sysconf(C._SC_CLK_TCK))
				c.cpu.With(prometheus.Labels{"cpu": parts[0], "mode": cpuFields[i]}).Set(value)
			}
		case parts[0] == "intr":
			// Only expose the overall number, use the 'interrupts' collector for more detail.
			value, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return err
			}
			c.intr.Set(value)
		case parts[0] == "ctxt":
			value, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return err
			}
			c.ctxt.Set(value)
		case parts[0] == "processes":
			value, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return err
			}
			c.forks.Set(value)
		case parts[0] == "btime":
			value, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return err
			}
			c.btime.Set(value)
		case parts[0] == "procs_running":
			value, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return err
			}
			c.procsRunning.Set(value)
		case parts[0] == "procs_blocked":
			value, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return err
			}
			c.procsBlocked.Set(value)
		}
	}
	c.cpu.Collect(ch)
	c.ctxt.Collect(ch)
	c.intr.Collect(ch)
	c.forks.Collect(ch)
	c.btime.Collect(ch)
	c.procsRunning.Collect(ch)
	c.procsBlocked.Collect(ch)
	return err
}
