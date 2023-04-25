package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type TimeUTC time.Time

const TimeUTCFormat = "2006-01-02T15:04:05.999999Z07:00"

func (t TimeUTC) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	parsed, err := time.Parse(TimeUTCFormat, s)
	if err != nil {
		return err
	}
	t = TimeUTC(parsed)
	return nil
}

const (
	namespace = "uls"
)

var (
	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"ULS is up and running",
		nil, nil,
	)
	lease = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "leases"),
		"Number of active ULS leases",
		nil, nil,
	)
)

type ULSClientEntitlementContext struct {
	EnvironmentDomain     string `json:"EnvironmentDomain"`
	EnvironmentHostname   string `json:"EnvironmentHostname"`
	EnvironmentUser       string `json:"EnvironmentUser"`
	LegacyMachineBinding1 string `json:"Legacy.MachineBinding1"`
	LegacyMachineBinding2 string `json:"Legacy.MachineBinding2"`
	LegacyMachineBinding5 string `json:"Legacy.MachineBinding5"`
}

type ULSLease struct {
	FloatingLeaseID          int                         `json:"floatingLeaseId"`
	Token                    uuid.UUID                   `json:"token"`
	CreatedTimeUTC           TimeUTC                     `json:"createdTimeUtc"`
	LastRenewalTimeUTC       TimeUTC                     `json:"lastRenewalTimeUtc"`
	IsRevoked                bool                        `json:"isRevoked"`
	ClientEntitlementContext ULSClientEntitlementContext `json:"clientEntitlementContext"`
	EntitlementGroupIDs      []string                    `json:"entitlementGroupIds"`
}

type ULSExporter struct {
	BaseURL *url.URL
}

func NewULSExporter(baseURL string) (*ULSExporter, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	return &ULSExporter{BaseURL: u}, nil
}

func (e *ULSExporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	ch <- lease
}

func (e *ULSExporter) Collect(ch chan<- prometheus.Metric) {
	leases, err := e.GetLeases()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 0)
		log.Println(err)
		return
	}
	ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(lease, prometheus.GaugeValue, float64(len(leases)))
}

func (e *ULSExporter) GetLeases() ([]ULSLease, error) {
	leaseURL, err := e.BaseURL.Parse("/v1/admin/lease")
	if err != nil {
		return nil, err
	}
	res, err := http.Get(leaseURL.String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%d %s", res.StatusCode, res.Status)
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var leases []ULSLease
	err = json.Unmarshal(b, &leases)
	if err != nil {
		return nil, err
	}
	return leases, nil
}

type App struct {
	Listen string
	Path   string
	URI    string
}

func (app *App) Main() error {
	flag.StringVar(&app.Listen, "listen", envDefault("ULS_LISTEN", ":9101"), "address to listen")
	flag.StringVar(&app.Path, "path", envDefault("ULS_PATH", "/metrics"), "path to export metrics")
	flag.StringVar(&app.URI, "uri", envDefault("ULS_URI", "http://localhost:8080"), "server base URI")
	flag.Parse()
	exporter, err := NewULSExporter(app.URI)
	if err != nil {
		return err
	}
	err = prometheus.Register(exporter)
	if err != nil {
		return err
	}
	http.Handle(app.Path, promhttp.Handler())
	return http.ListenAndServe(app.Listen, nil)
}

func envDefault(env string, def string) string {
	s, ok := os.LookupEnv(env)
	if ok {
		return s
	}
	return def
}

func main() {
	app := &App{}
	err := app.Main()
	if err != nil {
		log.Fatal(err)
	}
}
