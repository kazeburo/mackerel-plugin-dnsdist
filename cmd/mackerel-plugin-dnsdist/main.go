package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jessevdk/go-flags"
	mp "github.com/mackerelio/go-mackerel-plugin"
)

const (
	StatusCodeOK      = 0
	StatusCodeWARNING = 1
)

// version by Makefile
var version string

type Opt struct {
	Version bool   `short:"v" long:"version" description:"Show version"`
	Prefix  string `long:"prefix" default:"dnsdist" description:"Metric key prefix"`

	Port    string        `short:"p" long:"port" default:"8083" description:"Port number"`
	Host    string        `short:"H" long:"hostname" default:"127.0.0.1" description:"Hostname"`
	Timeout time.Duration `long:"timeout" default:"30s" description:"Timeout"`

	APIKey string `long:"api-key" description:"api key"`
}

func (o *Opt) URL() string {
	url := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort(o.Host, o.Port),
		Path:     "/jsonstat",
		RawQuery: "command=stats",
	}
	return url.String()
}

var apiKeyRegexp = regexp.MustCompile(`setWebserverConfig\(.*\{.*\bapiKey\s*=\s*"(.+?)"`)

func (o *Opt) GetAPIKey() string {
	if o.APIKey != "" {
		return o.APIKey
	}
	buf, err := os.ReadFile("/etc/dnsdist/dnsdist.conf")
	if err != nil {
		return ""
	}
	res := apiKeyRegexp.FindAllSubmatch(buf, -1)
	if len(res) < 1 {
		return ""
	}
	return string(res[0][1])
}

type Plugin struct {
	Prefix  string
	URL     string
	Timeout time.Duration
	APIKey  string
}

func (p *Plugin) httpClient() *http.Client {
	transport := &http.Transport{
		// inherited http.DefaultTransport
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   p.Timeout,
			KeepAlive: p.Timeout,
		}).DialContext,
		TLSHandshakeTimeout:   p.Timeout,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: p.Timeout,
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (p *Plugin) MetricKeyPrefix() string {
	if p.Prefix == "" {
		p.Prefix = "dnsdist"
	}
	return p.Prefix
}

func (p *Plugin) GraphDefinition() map[string]mp.Graphs {
	labelPrefix := strings.Title(p.Prefix)
	return map[string]mp.Graphs{
		"acl-drop": {
			Label: labelPrefix + ": Dropped packets becaused of the ACL",
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "acl-drops", Label: "Dropped", Diff: true},
			},
		},
		"cache": {
			Label: labelPrefix + ": Packet Cache",
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "cache-hits", Label: "Hits", Stacked: true, Diff: true},
				{Name: "cache-misses", Label: "Misses", Stacked: true, Diff: true},
			},
		},
		"downstream-errors": {
			Label: labelPrefix + ": Backend errors",
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "downstream-send-errors", Label: "Send error", Diff: true},
				{Name: "downstream-timeouts", Label: "Timeouts", Diff: true},
			},
		},
		"latency": {
			Label: labelPrefix + ": Latency (microseconds)",
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "latency-avg1000000", Label: "Latency1000000"},
			},
		},
		"queries": {
			Label: labelPrefix + ": Queries",
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "queries", Label: "Queries", Diff: true},
				{Name: "rdqueries", Label: "Query widh rd bit", Diff: true},
			},
		},
		"responses": {
			Label: labelPrefix + ": Response",
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "responses", Label: "Backend responses", Diff: true},
				{Name: "self-answered", Label: "Self answered", Diff: true},
				{Name: "servfail-responses", Label: "Backend servfail", Diff: true},
			},
		},
		"rule": {
			Label: labelPrefix + ": Returned because of rules",
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "rule-drop", Label: "Drop", Stacked: true, Diff: true},
				{Name: "rule-nxdomain", Label: "Nxdomain", Stacked: true, Diff: true},
				{Name: "rule-refused", Label: "Refused", Stacked: true, Diff: true},
				{Name: "rule-servfail", Label: "Servfail", Stacked: true, Diff: true},
				{Name: "rule-truncated", Label: "Truncated", Stacked: true, Diff: true},
			},
		},
		"fd": {
			Label: labelPrefix + ": FD usage",
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "fd-usage", Label: "usage"},
			},
		},
	}
}

func (p *Plugin) FetchMetrics() (map[string]float64, error) {
	req, err := http.NewRequest("GET", p.URL, nil)
	if err != nil {
		return nil, err
	}
	if p.APIKey != "" {
		req.Header.Add("X-API-Key", p.APIKey)
	}
	res, err := p.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	t := map[string]interface{}{}
	decoder := json.NewDecoder(res.Body)
	decoder.UseNumber()

	if err := decoder.Decode(&t); err != nil {
		return nil, err
	}

	result := map[string]float64{}
	for k, b := range t {
		f, err := strconv.ParseFloat(fmt.Sprintf("%v", b), 64)
		if err != nil {
			continue
		}
		result[k] = f
	}
	return result, nil
}

func (u *Plugin) Run() {
	plugin := mp.NewMackerelPlugin(u)
	plugin.Run()
}

func main() {
	opt := Opt{}
	psr := flags.NewParser(&opt, flags.HelpFlag|flags.PassDoubleDash)
	_, err := psr.Parse()
	if opt.Version {
		fmt.Printf(`%s %s
Compiler: %s %s
`,
			os.Args[0],
			version,
			runtime.Compiler,
			runtime.Version())
		os.Exit(StatusCodeOK)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(StatusCodeWARNING)
	}

	u := &Plugin{
		Prefix:  opt.Prefix,
		Timeout: opt.Timeout,
		URL:     opt.URL(),
		APIKey:  opt.GetAPIKey(),
	}
	u.Run()
}
