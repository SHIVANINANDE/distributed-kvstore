package monitoring

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"
)

// Dashboard represents the monitoring dashboard
type Dashboard struct {
	healthManager   *HealthManager
	metricsRegistry *MetricsRegistry
	kvMetrics       *KVStoreMetrics
}

// NewDashboard creates a new monitoring dashboard
func NewDashboard(healthManager *HealthManager, metricsRegistry *MetricsRegistry, kvMetrics *KVStoreMetrics) *Dashboard {
	return &Dashboard{
		healthManager:   healthManager,
		metricsRegistry: metricsRegistry,
		kvMetrics:       kvMetrics,
	}
}

// DashboardData contains data for the dashboard template
type DashboardData struct {
	Title        string
	RefreshTime  string
	Health       HealthResponse
	Metrics      map[string]*Metric
	SystemStats  SystemStats
	ChartData    ChartData
	Alerts       []Alert
	ServiceInfo  ServiceInfo
}

// SystemStats represents system statistics
type SystemStats struct {
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsageMB  uint64  `json:"memory_usage_mb"`
	GoroutineCount int     `json:"goroutine_count"`
	GCPause        float64 `json:"gc_pause_ms"`
	Uptime         string  `json:"uptime"`
}

// ChartData contains data for dashboard charts
type ChartData struct {
	RequestsPerSecond []TimeValue `json:"requests_per_second"`
	ResponseTimes     []TimeValue `json:"response_times"`
	ErrorRates        []TimeValue `json:"error_rates"`
	StorageSize       []TimeValue `json:"storage_size"`
}

// TimeValue represents a time-series data point
type TimeValue struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

// Alert represents a monitoring alert
type Alert struct {
	Level       string    `json:"level"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Source      string    `json:"source"`
}

// ServiceInfo contains service information
type ServiceInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GitCommit string `json:"git_commit"`
}

// ServeHTTP implements the dashboard HTTP handler
func (d *Dashboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/dashboard":
		d.serveDashboardHTML(w, r)
	case "/dashboard/api/health":
		d.serveHealthAPI(w, r)
	case "/dashboard/api/metrics":
		d.serveMetricsAPI(w, r)
	case "/dashboard/api/alerts":
		d.serveAlertsAPI(w, r)
	case "/dashboard/api/charts":
		d.serveChartsAPI(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (d *Dashboard) serveDashboardHTML(w http.ResponseWriter, r *http.Request) {
	data := d.getDashboardData()
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	tmpl, err := template.New("dashboard").Parse(dashboardHTML)
	if err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
		return
	}
	
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("Template execution error: %v", err), http.StatusInternalServerError)
		return
	}
}

func (d *Dashboard) serveHealthAPI(w http.ResponseWriter, r *http.Request) {
	health := d.healthManager.CheckHealth(r.Context())
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (d *Dashboard) serveMetricsAPI(w http.ResponseWriter, r *http.Request) {
	d.kvMetrics.UpdateSystemMetrics()
	metrics := d.metricsRegistry.GetAllMetrics()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (d *Dashboard) serveAlertsAPI(w http.ResponseWriter, r *http.Request) {
	alerts := d.generateAlerts()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

func (d *Dashboard) serveChartsAPI(w http.ResponseWriter, r *http.Request) {
	chartData := d.generateChartData()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chartData)
}

func (d *Dashboard) getDashboardData() DashboardData {
	health := d.healthManager.CheckHealth(nil)
	d.kvMetrics.UpdateSystemMetrics()
	metrics := d.metricsRegistry.GetAllMetrics()
	
	return DashboardData{
		Title:       "KVStore Monitoring Dashboard",
		RefreshTime: time.Now().Format("15:04:05"),
		Health:      health,
		Metrics:     metrics,
		SystemStats: d.getSystemStats(),
		ChartData:   d.generateChartData(),
		Alerts:      d.generateAlerts(),
		ServiceInfo: ServiceInfo{
			Name:      "Distributed Key-Value Store",
			Version:   "1.0.0",
			BuildTime: time.Now().Format("2006-01-02 15:04:05"),
			GitCommit: "main",
		},
	}
}

func (d *Dashboard) getSystemStats() SystemStats {
	memUsage := d.kvMetrics.MemoryUsage.Get()
	goroutineCount := int(d.kvMetrics.GoroutineCount.Get())
	
	return SystemStats{
		CPUUsage:       0.0, // Would be calculated from system metrics
		MemoryUsageMB:  uint64(memUsage / 1024 / 1024),
		GoroutineCount: goroutineCount,
		GCPause:        0.0, // Would be calculated from GC metrics
		Uptime:         "running",
	}
}

func (d *Dashboard) generateChartData() ChartData {
	now := time.Now()
	
	// Generate sample time series data (in a real implementation, this would come from stored metrics)
	var requestsPerSecond, responseTimes, errorRates, storageSize []TimeValue
	
	for i := 0; i < 20; i++ {
		t := now.Add(time.Duration(-i*30) * time.Second)
		timeStr := t.Format("15:04:05")
		
		requestsPerSecond = append(requestsPerSecond, TimeValue{
			Time:  timeStr,
			Value: float64(d.kvMetrics.RequestsTotal.Get()),
		})
		
		responseTimes = append(responseTimes, TimeValue{
			Time:  timeStr,
			Value: 0.1 + float64(i)*0.01, // Sample response time
		})
		
		errorRates = append(errorRates, TimeValue{
			Time:  timeStr,
			Value: float64(d.kvMetrics.RequestErrors.Get()),
		})
		
		storageSize = append(storageSize, TimeValue{
			Time:  timeStr,
			Value: d.kvMetrics.StorageSize.Get(),
		})
	}
	
	return ChartData{
		RequestsPerSecond: requestsPerSecond,
		ResponseTimes:     responseTimes,
		ErrorRates:        errorRates,
		StorageSize:       storageSize,
	}
}

func (d *Dashboard) generateAlerts() []Alert {
	var alerts []Alert
	
	health := d.healthManager.CheckHealth(nil)
	
	// Generate alerts based on health checks
	for name, check := range health.Checks {
		if check.Status != HealthStatusHealthy {
			level := "warning"
			if check.Status == HealthStatusUnhealthy {
				level = "error"
			}
			
			alerts = append(alerts, Alert{
				Level:       level,
				Title:       fmt.Sprintf("%s Health Check Failed", name),
				Description: check.Message,
				Timestamp:   check.Timestamp,
				Source:      "health_check",
			})
		}
	}
	
	// Generate alerts based on metrics
	memUsage := d.kvMetrics.MemoryUsage.Get() / 1024 / 1024 // Convert to MB
	if memUsage > 500 { // 500MB threshold
		alerts = append(alerts, Alert{
			Level:       "warning",
			Title:       "High Memory Usage",
			Description: fmt.Sprintf("Memory usage is %.0f MB", memUsage),
			Timestamp:   time.Now(),
			Source:      "metrics",
		})
	}
	
	goroutines := d.kvMetrics.GoroutineCount.Get()
	if goroutines > 1000 {
		alerts = append(alerts, Alert{
			Level:       "warning",
			Title:       "High Goroutine Count",
			Description: fmt.Sprintf("Goroutine count is %.0f", goroutines),
			Timestamp:   time.Now(),
			Source:      "metrics",
		})
	}
	
	// Sort alerts by timestamp (newest first)
	sort.Slice(alerts, func(i, j int) bool {
		return alerts[i].Timestamp.After(alerts[j].Timestamp)
	})
	
	return alerts
}

// HTML template for the dashboard
const dashboardHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: #f5f5f5;
            color: #333;
        }
        
        .header {
            background: #2c3e50;
            color: white;
            padding: 1rem 2rem;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        
        .header h1 {
            font-size: 1.5rem;
        }
        
        .refresh-info {
            font-size: 0.9rem;
            opacity: 0.8;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 2rem;
        }
        
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 1.5rem;
            margin-bottom: 2rem;
        }
        
        .card {
            background: white;
            border-radius: 8px;
            padding: 1.5rem;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        
        .card h3 {
            margin-bottom: 1rem;
            color: #2c3e50;
            border-bottom: 2px solid #ecf0f1;
            padding-bottom: 0.5rem;
        }
        
        .health-status {
            display: inline-block;
            padding: 0.25rem 0.5rem;
            border-radius: 4px;
            font-size: 0.8rem;
            font-weight: bold;
            text-transform: uppercase;
        }
        
        .health-healthy { background: #2ecc71; color: white; }
        .health-degraded { background: #f39c12; color: white; }
        .health-unhealthy { background: #e74c3c; color: white; }
        
        .metric-item {
            display: flex;
            justify-content: space-between;
            padding: 0.5rem 0;
            border-bottom: 1px solid #ecf0f1;
        }
        
        .metric-item:last-child {
            border-bottom: none;
        }
        
        .metric-value {
            font-weight: bold;
            color: #2980b9;
        }
        
        .alert {
            padding: 1rem;
            margin-bottom: 1rem;
            border-radius: 4px;
            border-left: 4px solid;
        }
        
        .alert-error {
            background: #fdf2f2;
            border-color: #e74c3c;
            color: #721c24;
        }
        
        .alert-warning {
            background: #fefbf2;
            border-color: #f39c12;
            color: #8b5a2b;
        }
        
        .alert-info {
            background: #f2f8fd;
            border-color: #3498db;
            color: #2980b9;
        }
        
        .chart-container {
            height: 200px;
            background: #f8f9fa;
            border-radius: 4px;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #666;
        }
        
        .system-stat {
            text-align: center;
            padding: 1rem;
        }
        
        .system-stat-value {
            font-size: 2rem;
            font-weight: bold;
            color: #2980b9;
        }
        
        .system-stat-label {
            font-size: 0.9rem;
            color: #666;
            margin-top: 0.5rem;
        }
        
        .full-width {
            grid-column: 1 / -1;
        }
        
        .refresh-button {
            background: #3498db;
            color: white;
            border: none;
            padding: 0.5rem 1rem;
            border-radius: 4px;
            cursor: pointer;
            font-size: 0.9rem;
        }
        
        .refresh-button:hover {
            background: #2980b9;
        }
        
        .service-info {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 1rem;
        }
        
        .info-item {
            display: flex;
            flex-direction: column;
        }
        
        .info-label {
            font-size: 0.8rem;
            color: #666;
            margin-bottom: 0.25rem;
        }
        
        .info-value {
            font-weight: bold;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>{{.Title}}</h1>
        <div class="refresh-info">
            Last updated: {{.RefreshTime}}
            <button class="refresh-button" onclick="location.reload()">Refresh</button>
        </div>
    </div>
    
    <div class="container">
        <!-- Health Status -->
        <div class="grid">
            <div class="card">
                <h3>Health Status</h3>
                <div>
                    <span class="health-status health-{{.Health.Status}}">{{.Health.Status}}</span>
                    <div style="margin-top: 1rem;">
                        <div class="metric-item">
                            <span>Uptime</span>
                            <span class="metric-value">{{printf "%.2f" .Health.Uptime.Seconds}}s</span>
                        </div>
                        <div class="metric-item">
                            <span>Total Checks</span>
                            <span class="metric-value">{{.Health.Summary.Total}}</span>
                        </div>
                        <div class="metric-item">
                            <span>Healthy</span>
                            <span class="metric-value" style="color: #2ecc71;">{{.Health.Summary.Healthy}}</span>
                        </div>
                        {{if gt .Health.Summary.Degraded 0}}
                        <div class="metric-item">
                            <span>Degraded</span>
                            <span class="metric-value" style="color: #f39c12;">{{.Health.Summary.Degraded}}</span>
                        </div>
                        {{end}}
                        {{if gt .Health.Summary.Unhealthy 0}}
                        <div class="metric-item">
                            <span>Unhealthy</span>
                            <span class="metric-value" style="color: #e74c3c;">{{.Health.Summary.Unhealthy}}</span>
                        </div>
                        {{end}}
                    </div>
                </div>
            </div>
            
            <!-- System Stats -->
            <div class="card">
                <h3>System Statistics</h3>
                <div style="display: grid; grid-template-columns: repeat(2, 1fr); gap: 1rem;">
                    <div class="system-stat">
                        <div class="system-stat-value">{{.SystemStats.MemoryUsageMB}}</div>
                        <div class="system-stat-label">Memory (MB)</div>
                    </div>
                    <div class="system-stat">
                        <div class="system-stat-value">{{.SystemStats.GoroutineCount}}</div>
                        <div class="system-stat-label">Goroutines</div>
                    </div>
                </div>
            </div>
            
            <!-- Service Information -->
            <div class="card">
                <h3>Service Information</h3>
                <div class="service-info">
                    <div class="info-item">
                        <span class="info-label">Name</span>
                        <span class="info-value">{{.ServiceInfo.Name}}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Version</span>
                        <span class="info-value">{{.ServiceInfo.Version}}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Build Time</span>
                        <span class="info-value">{{.ServiceInfo.BuildTime}}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Git Commit</span>
                        <span class="info-value">{{.ServiceInfo.GitCommit}}</span>
                    </div>
                </div>
            </div>
        </div>
        
        <!-- Metrics Overview -->
        <div class="grid">
            <div class="card full-width">
                <h3>Key Metrics</h3>
                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem;">
                    {{range $name, $metric := .Metrics}}
                    <div class="metric-item">
                        <span>{{$name}}</span>
                        <span class="metric-value">{{printf "%.2f" $metric.Value}}</span>
                    </div>
                    {{end}}
                </div>
            </div>
        </div>
        
        <!-- Alerts -->
        {{if .Alerts}}
        <div class="card">
            <h3>Alerts ({{len .Alerts}})</h3>
            {{range .Alerts}}
            <div class="alert alert-{{.Level}}">
                <strong>{{.Title}}</strong><br>
                {{.Description}}<br>
                <small>{{.Timestamp.Format "15:04:05"}} - {{.Source}}</small>
            </div>
            {{end}}
        </div>
        {{end}}
        
        <!-- Charts (Placeholder) -->
        <div class="grid">
            <div class="card">
                <h3>Request Rate</h3>
                <div class="chart-container">
                    Chart: Requests per second over time
                </div>
            </div>
            <div class="card">
                <h3>Response Times</h3>
                <div class="chart-container">
                    Chart: Response time percentiles
                </div>
            </div>
        </div>
    </div>
    
    <script>
        // Auto-refresh every 30 seconds
        setTimeout(() => location.reload(), 30000);
    </script>
</body>
</html>
`