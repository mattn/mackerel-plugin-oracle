package mporacle

import (
	"bytes"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"regexp"
	"strings"

	mp "github.com/mackerelio/go-mackerel-plugin-helper"
	"github.com/mackerelio/golib/logging"
	_ "github.com/mattn/go-oci8"
)

var logger = logging.GetLogger("metrics.plugin.oracle")

type waitEventName struct {
	Name    string
	Pattern *regexp.Regexp
}

type waitEventNames []waitEventName

var optWaitEvents waitEventNames

func (we *waitEventNames) Match(name string) bool {
	for _, w := range *we {
		if w.Pattern != nil && w.Pattern.MatchString(name) {
			return true
		}
		if w.Name == name {
			return true
		}
	}
	return false
}

func (we *waitEventNames) String() string {
	var buf bytes.Buffer
	for i, w := range *we {
		if i > 0 {
			buf.WriteString(",")
		}
		fmt.Fprintf(&buf, "%q", w.Name)
	}
	return buf.String()
}

func (we *waitEventNames) Set(value string) error {
	if value == "" {
		return errors.New("event name must not be empty")
	}
	var w waitEventName
	w.Name = value
	if len(value) > 2 && value[0] == '/' && value[len(value)-1] == '/' {
		var err error
		w.Pattern, err = regexp.Compile(value[1 : len(value)-2])
		if err != nil {
			return err
		}
	}
	*we = append(*we, w)
	return nil
}

// OraclePlugin mackerel plugin for Oracle
type OraclePlugin struct {
	Prefix string
	DSN    string
}

var replacer = strings.NewReplacer(
	"/", "",
	" ", "_",
	"*", "_",
	":", "_",
)

func normalize(s string) string {
	return strings.ToLower(replacer.Replace(s))
}

func fetchResource(db *sql.DB) (map[string]interface{}, error) {
	rows, err := db.Query(`
    select resource_name, current_utilization
	from v$resource_limit
	where resource_name = 'processes' or resource_name = 'sessions'
	`)
	if err != nil {
		logger.Errorf("Failed to select resource. %s", err)
		return nil, err
	}

	stat := make(map[string]interface{})

	for rows.Next() {
		var name string
		var curr float64
		err = rows.Scan(&name, &curr)
		if err != nil {
			return nil, err
		}
		stat[normalize(name)] = curr
	}

	return stat, nil
}

func fetchWaitClass(db *sql.DB) (map[string]interface{}, error) {
	rows, err := db.Query(`
	select n.wait_class, round(m.time_waited/m.INTSIZE_CSEC,3) AAS
	from v$waitclassmetric m, v$system_wait_class n
	where m.wait_class_id=n.wait_class_id and n.wait_class != 'Idle'
	union
	select  'CPU', round(value/100,3) AAS
	from v$sysmetric where metric_name='CPU Usage Per Sec' and group_id=2
	union select 'CPU_OS', round((prcnt.busy*parameter.cpu_count)/100,3) - aas.cpu
	from
	( select value busy
	    from v$sysmetric
	    where metric_name='Host CPU Utilization (%)'
	    and group_id=2 ) prcnt,
	    ( select value cpu_count from v$parameter where name='cpu_count' )  parameter,
	    ( select  'CPU', round(value/100,3) cpu from v$sysmetric where metric_name='CPU Usage Per Sec' and group_id=2) aas
	`)
	if err != nil {
		logger.Errorf("Failed to select wait_class. %s", err)
		return nil, err
	}

	stat := make(map[string]interface{})

	for rows.Next() {
		var class string
		var ass float64
		err = rows.Scan(&class, &ass)
		if err != nil {
			return nil, err
		}
		stat[normalize(class)] = ass
	}

	return stat, nil
}

func fetchWaitEvents(db *sql.DB) (map[string]interface{}, error) {
	stat := make(map[string]interface{})

	if len(optWaitEvents) == 0 {
		return stat, nil
	}

	rows, err := db.Query(`
	select
	    n.wait_class wait_class,
		n.name wait_name,
		m.wait_count cnt,
		round(10*m.time_waited/nullif(m.wait_count,0),3) avgms
	    from v$eventmetric m,
		v$event_name n
	    where m.event_id=n.event_id
	    and n.wait_class <> 'Idle' and m.wait_count > 0 order by 1
	`)
	if err != nil {
		logger.Errorf("Failed to select wait_event. %s", err)
		return nil, err
	}

	for rows.Next() {
		var class, name string
		var cnt, avgms float64
		err = rows.Scan(&class, &name, &cnt, &avgms)
		if err != nil {
			return nil, err
		}
		logger.Infof("Event %s.%s: count=%f, latency=%f", class, name, cnt, avgms)
		if optWaitEvents.Match(name) {
			stat[normalize(name)+"_count"] = cnt
			stat[normalize(name)+"_latency"] = avgms
		}
	}

	return stat, nil
}

func mergeStat(dst, src map[string]interface{}) {
	for k, v := range src {
		dst[k] = v
	}
}

// MetricKeyPrefix retruns the metrics key prefix
func (p OraclePlugin) MetricKeyPrefix() string {
	if p.Prefix == "" {
		p.Prefix = "oracle"
	}
	return p.Prefix
}

// FetchMetrics interface for mackerelplugin
func (p OraclePlugin) FetchMetrics() (map[string]interface{}, error) {
	db, err := sql.Open("oci8", p.DSN)
	if err != nil {
		logger.Errorf("FetchMetrics: %s", err)
		return nil, err
	}
	defer db.Close()

	statResource, err := fetchResource(db)
	if err != nil {
		return nil, err
	}

	statWaitClass, err := fetchWaitClass(db)
	if err != nil {
		return nil, err
	}

	statWaitEvents, err := fetchWaitEvents(db)
	if err != nil {
		return nil, err
	}

	stat := make(map[string]interface{})

	mergeStat(stat, statResource)
	mergeStat(stat, statWaitClass)
	mergeStat(stat, statWaitEvents)

	return stat, err
}

// GraphDefinition interface for mackerelplugin
func (p OraclePlugin) GraphDefinition() map[string]mp.Graphs {
	labelPrefix := strings.Title(p.MetricKeyPrefix())

	var graphdef = map[string]mp.Graphs{
		"resource": {
			Label: (labelPrefix + " Resource Limit"),
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "processes", Label: "Processes", Diff: false, Stacked: false},
				{Name: "sessions", Label: "Sessions", Diff: false, Stacked: false},
			},
		},
		"waitclass": {
			Label: (labelPrefix + " Wait Class"),
			Unit:  "float",
			Metrics: []mp.Metrics{
				{Name: "administrative", Label: "Administrative", Diff: false, Stacked: false},
				{Name: "cpu", Label: "CPU", Diff: false, Stacked: false},
				{Name: "cpu_os", Label: "CPU/OS", Diff: false, Stacked: false},
				{Name: "concurrency", Label: "Concurrency", Diff: false, Stacked: false},
				{Name: "configuration", Label: "Configuration", Diff: false, Stacked: false},
				{Name: "network", Label: "Network", Diff: false, Stacked: false},
				{Name: "other", Label: "Other", Diff: false, Stacked: false},
				{Name: "scheduler", Label: "Scheduler", Diff: false, Stacked: false},
			},
		},
	}

	for _, e := range optWaitEvents {
		name := normalize(e.Name)
		graphdef[name] = mp.Graphs{
			Label: (labelPrefix + " Wait Events: " + e.Name),
			Unit:  "float",
			Metrics: []mp.Metrics{
				{Name: name + "_count", Label: "Count", Diff: false, Stacked: false},
				{Name: name + "_latency", Label: "Latency", Diff: false, Stacked: false},
			},
		}
	}

	return graphdef
}

// Do the plugin
func Do() {
	optDSN := flag.String("dsn", "system/manager", "Database Source Name")
	optPrefix := flag.String("metric-key-prefix", "oracle", "Metric key prefix")
	optTempfile := flag.String("tempfile", "", "Temp file name")
	flag.Var(&optWaitEvents, "event", "List of WaitEvent name")
	flag.Parse()

	var oracle OraclePlugin
	oracle.DSN = *optDSN
	oracle.Prefix = *optPrefix

	helper := mp.NewMackerelPlugin(oracle)

	helper.Tempfile = *optTempfile
	helper.Run()
}
