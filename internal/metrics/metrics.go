package metrics

import (
	"database/sql"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/chia-network/go-chia-libs/pkg/rpc"
	wrappedPrometheus "github.com/chia-network/go-modules/pkg/prometheus"
	"github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
)

// prometheusMetrics is the struct with metrics that holds the actual prometheus metric objects
type prometheusMetrics struct {
	nakamotoCoefficient50 *wrappedPrometheus.LazyGauge
	nakamotoCoefficient51 *wrappedPrometheus.LazyGauge

	nakamotoCoefficient50Adjusted *wrappedPrometheus.LazyGauge
	nakamotoCoefficient51Adjusted *wrappedPrometheus.LazyGauge

	blockHeight *wrappedPrometheus.LazyGauge
}

// Metrics deals with the block db and metrics
type Metrics struct {
	exporterPort uint16

	dbHost string
	dbPort uint16
	dbUser string
	dbPass string
	dbName string

	websocketClient *rpc.Client

	mysqlClient *sql.DB

	// This holds a custom prometheus registry so that only our metrics are exported, and not the default go metrics
	registry          *prometheus.Registry
	prometheusMetrics *prometheusMetrics

	lookbackWindow uint32
	rpcPerPage     uint32

	refreshing  *sync.Mutex
	peakLock    *sync.Mutex
	highestPeak uint32

	fillGapsLock *sync.Mutex
}

// NewMetrics returns a new metrics instance
func NewMetrics(exporterPort uint16, dbHost string, dbPort uint16, dbUser string, dbPass string, dbName string, lookbackWindow int, rpcPerPage int) (*Metrics, error) {
	var err error

	metrics := &Metrics{
		exporterPort:      exporterPort,
		dbHost:            dbHost,
		dbPort:            dbPort,
		dbUser:            dbUser,
		dbPass:            dbPass,
		dbName:            dbName,
		registry:          prometheus.NewRegistry(),
		prometheusMetrics: &prometheusMetrics{},
		lookbackWindow:    uint32(lookbackWindow),
		rpcPerPage:        uint32(rpcPerPage),
		refreshing:        &sync.Mutex{},
		peakLock:          &sync.Mutex{},
		fillGapsLock:      &sync.Mutex{},
	}

	metrics.websocketClient, err = rpc.NewClient(rpc.ConnectionModeWebsocket, rpc.WithAutoConfig(), rpc.WithSyncWebsocket(), rpc.WithBaseURL(&url.URL{
		Scheme: "wss",
		Host:   viper.GetString("chia-hostname"),
	}))
	if err != nil {
		return nil, err
	}

	err = metrics.createDBClient()
	if err != nil {
		return nil, err
	}

	err = metrics.initTables()
	if err != nil {
		return nil, err
	}

	metrics.initMetrics()

	return metrics, nil
}

func (m *Metrics) createDBClient() error {
	var err error

	cfg := mysql.Config{
		User:                 m.dbUser,
		Passwd:               m.dbPass,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", m.dbHost, m.dbPort),
		DBName:               m.dbName,
		AllowNativePasswords: true,
	}
	m.mysqlClient, err = sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}

	m.mysqlClient.SetConnMaxLifetime(time.Minute * 3)
	m.mysqlClient.SetMaxOpenConns(10)
	m.mysqlClient.SetMaxIdleConns(10)

	return nil
}

func (m *Metrics) initMetrics() {
	m.prometheusMetrics.nakamotoCoefficient50 = m.newGauge("nakamoto_coefficient_gt50", "Nakamoto coefficient when we calculate for >50% of nodes")
	m.prometheusMetrics.nakamotoCoefficient51 = m.newGauge("nakamoto_coefficient_gt51", "Nakamoto coefficient when we calculate for >51% of nodes")
	m.prometheusMetrics.nakamotoCoefficient50Adjusted = m.newGauge("nakamoto_coefficient_gt50_adjusted", "Nakamoto coefficient when we calculate for >50% of nodes excluding configured farmer addresses")
	m.prometheusMetrics.nakamotoCoefficient51Adjusted = m.newGauge("nakamoto_coefficient_gt51_adjusted", "Nakamoto coefficient when we calculate for >51% of nodes excluding configured farmer addresses")
	m.prometheusMetrics.blockHeight = m.newGauge("block_height", "Block height for current set of metrics")
}

// newGauge returns a lazy gauge that follows naming conventions
func (m *Metrics) newGauge(name string, help string) *wrappedPrometheus.LazyGauge {
	opts := prometheus.GaugeOpts{
		Namespace: "chia",
		Subsystem: "block_metrics",
		Name:      name,
		Help:      help,
	}

	gm := prometheus.NewGauge(opts)

	lg := &wrappedPrometheus.LazyGauge{
		Gauge:    gm,
		Registry: m.registry,
	}

	return lg
}

// LookbackWindow returns the configured lookback window
func (m *Metrics) LookbackWindow() uint32 {
	return m.lookbackWindow
}
