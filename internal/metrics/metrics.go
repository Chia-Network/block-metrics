package metrics

import (
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/chia-network/go-chia-libs/pkg/rpc"
	wrappedPrometheus "github.com/chia-network/go-modules/pkg/prometheus"
	"github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
)

// Metrics deals with the block db and metrics
type Metrics struct {
	exporterPort uint16

	dbHost string
	dbPort uint16
	dbUser string
	dbPass string
	dbName string

	websocketClient *rpc.Client
	httpClient      *rpc.Client

	mysqlClient *sql.DB

	// This holds a custom prometheus registry so that only our metrics are exported, and not the default go metrics
	registry *prometheus.Registry
}

// NewMetrics returns a new metrics instance
func NewMetrics(exporterPort uint16, dbHost string, dbPort uint16, dbUser string, dbPass string, dbName string) (*Metrics, error) {
	var err error

	metrics := &Metrics{
		exporterPort: exporterPort,
		dbHost:       dbHost,
		dbPort:       dbPort,
		dbUser:       dbUser,
		dbPass:       dbPass,
		dbName:       dbName,
		registry:     prometheus.NewRegistry(),
	}

	metrics.websocketClient, err = rpc.NewClient(rpc.ConnectionModeWebsocket, rpc.WithAutoConfig(), rpc.WithBaseURL(&url.URL{
		Scheme: "wss",
		Host:   viper.GetString("chia-hostname"),
	}))
	if err != nil {
		return nil, err
	}

	metrics.httpClient, err = rpc.NewClient(rpc.ConnectionModeHTTP, rpc.WithAutoConfig(), rpc.WithBaseURL(&url.URL{
		Scheme: "https",
		Host:   viper.GetString("chia-hostname"),
	}))
	if err != nil {
		return nil, err
	}

	err = metrics.createDBClient()
	if err != nil {
		return nil, err
	}

	metrics.initMetrics()

	return metrics, nil
}

func (m *Metrics) createDBClient() error {
	var err error

	cfg := mysql.Config{
		User:   m.dbUser,
		Passwd: m.dbPass,
		Net:    "tcp",
		Addr:   fmt.Sprintf("%s:%d", m.dbHost, m.dbPort),
		DBName: m.dbName,
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
	m.newGauge("nakamoto_coefficient_gt50", "Nakamoto coefficient when we calculate for >50% of nodes")
	m.newGauge("nakamoto_coefficient_gt51", "Nakamoto coefficient when we calculate for >51% of nodes")
	m.newGauge("block_height", "Block height for current set of metrics")
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
