package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func main() {
	fmt.Println("=== KVStore Monitoring Demo ===")
	fmt.Println("This demo showcases the monitoring capabilities of the KVStore")
	fmt.Println()

	baseURL := "http://localhost:8080"

	// Wait a moment for server to be ready
	fmt.Println("Waiting for server to be ready...")
	waitForServer(baseURL)

	// Demo 1: Basic Health Check
	fmt.Println("1. Basic Health Check")
	fmt.Println("-------------------")
	testHealthCheck(baseURL)
	fmt.Println()

	// Demo 2: Generate some load to create metrics
	fmt.Println("2. Generating Load for Metrics")
	fmt.Println("-----------------------------")
	generateLoad(baseURL)
	fmt.Println()

	// Demo 3: Check Prometheus Metrics
	fmt.Println("3. Prometheus Metrics")
	fmt.Println("--------------------")
	testPrometheusMetrics(baseURL)
	fmt.Println()

	// Demo 4: Enhanced Health Check with Dependencies
	fmt.Println("4. Enhanced Health Check")
	fmt.Println("-----------------------")
	testEnhancedHealthCheck(baseURL)
	fmt.Println()

	// Demo 5: Monitoring Dashboard
	fmt.Println("5. Monitoring Dashboard")
	fmt.Println("----------------------")
	testMonitoringDashboard(baseURL)
	fmt.Println()

	fmt.Println("Demo completed! Check the following endpoints:")
	fmt.Printf("- Health Check: %s/health\n", baseURL)
	fmt.Printf("- Prometheus Metrics: %s/api/v1/metrics\n", baseURL)
	fmt.Printf("- Monitoring Dashboard: %s/dashboard\n", baseURL)
}

func waitForServer(baseURL string) {
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 10; i++ {
		resp, err := client.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			fmt.Println("✅ Server is ready!")
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
		fmt.Printf("⏳ Waiting for server... (%d/10)\n", i+1)
	}
	fmt.Println("⚠️  Server may not be ready, continuing anyway...")
}

func testHealthCheck(baseURL string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		fmt.Printf("❌ Health check failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read health response: %v\n", err)
		return
	}

	fmt.Printf("Status Code: %d\n", resp.StatusCode)
	
	// Parse and pretty-print JSON
	var healthData map[string]interface{}
	if err := json.Unmarshal(body, &healthData); err == nil {
		prettyJSON, _ := json.MarshalIndent(healthData, "  ", "  ")
		fmt.Printf("Response:\n%s\n", string(prettyJSON))
		
		// Extract key information
		if status, ok := healthData["status"].(string); ok {
			switch status {
			case "healthy":
				fmt.Println("✅ System is healthy")
			case "degraded":
				fmt.Println("⚠️  System is degraded")
			case "unhealthy":
				fmt.Println("❌ System is unhealthy")
			}
		}
		
		if summary, ok := healthData["summary"].(map[string]interface{}); ok {
			if total, ok := summary["total"].(float64); ok {
				fmt.Printf("Total health checks: %.0f\n", total)
			}
			if healthy, ok := summary["healthy"].(float64); ok {
				fmt.Printf("Healthy checks: %.0f\n", healthy)
			}
		}
	} else {
		fmt.Printf("Raw Response: %s\n", string(body))
	}
}

func generateLoad(baseURL string) {
	client := &http.Client{Timeout: 5 * time.Second}
	
	fmt.Println("Generating sample requests to create metrics...")
	
	operations := []struct {
		method string
		path   string
		body   string
	}{
		{"PUT", "/api/v1/kv/demo-key-1", `{"value": "demo-value-1"}`},
		{"PUT", "/api/v1/kv/demo-key-2", `{"value": "demo-value-2"}`},
		{"GET", "/api/v1/kv/demo-key-1", ""},
		{"GET", "/api/v1/kv/demo-key-2", ""},
		{"GET", "/api/v1/kv/nonexistent", ""}, // This will generate a 404
		{"PUT", "/api/v1/kv/demo-key-3", `{"value": "demo-value-3"}`},
		{"DELETE", "/api/v1/kv/demo-key-1", ""},
		{"GET", "/api/v1/kv", ""}, // List keys
	}
	
	successCount := 0
	errorCount := 0
	
	for _, op := range operations {
		var req *http.Request
		var err error
		
		if op.body != "" {
			req, err = http.NewRequest(op.method, baseURL+op.path, bytes.NewBufferString(op.body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, err = http.NewRequest(op.method, baseURL+op.path, nil)
		}
		
		if err != nil {
			fmt.Printf("❌ Failed to create request: %v\n", err)
			continue
		}
		
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("❌ Request failed: %v\n", err)
			errorCount++
			continue
		}
		resp.Body.Close()
		
		if resp.StatusCode < 400 {
			successCount++
			fmt.Printf("✅ %s %s -> %d\n", op.method, op.path, resp.StatusCode)
		} else {
			errorCount++
			fmt.Printf("⚠️  %s %s -> %d\n", op.method, op.path, resp.StatusCode)
		}
		
		// Small delay between requests
		time.Sleep(100 * time.Millisecond)
	}
	
	fmt.Printf("\nLoad generation completed: %d successful, %d errors\n", successCount, errorCount)
}

func testPrometheusMetrics(baseURL string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/api/v1/metrics")
	if err != nil {
		fmt.Printf("❌ Metrics endpoint failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read metrics response: %v\n", err)
		return
	}

	fmt.Printf("Status Code: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	
	// Parse and show some key metrics
	metrics := string(body)
	fmt.Println("\nSample Metrics (first 1000 characters):")
	if len(metrics) > 1000 {
		fmt.Printf("%s...\n", metrics[:1000])
	} else {
		fmt.Printf("%s\n", metrics)
	}
	
	// Count metric types
	lines := strings.Split(metrics, "\n")
	helpCount := 0
	typeCount := 0
	metricCount := 0
	
	for _, line := range lines {
		if strings.HasPrefix(line, "# HELP") {
			helpCount++
		} else if strings.HasPrefix(line, "# TYPE") {
			typeCount++
		} else if line != "" && !strings.HasPrefix(line, "#") {
			metricCount++
		}
	}
	
	fmt.Printf("\nMetrics Summary:\n")
	fmt.Printf("- Help lines: %d\n", helpCount)
	fmt.Printf("- Type definitions: %d\n", typeCount)
	fmt.Printf("- Metric values: %d\n", metricCount)
}

func testEnhancedHealthCheck(baseURL string) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(baseURL + "/api/v1/health")
	if err != nil {
		fmt.Printf("❌ Enhanced health check failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read health response: %v\n", err)
		return
	}

	var healthData map[string]interface{}
	if err := json.Unmarshal(body, &healthData); err != nil {
		fmt.Printf("❌ Failed to parse health JSON: %v\n", err)
		return
	}

	fmt.Printf("Overall Status: %s\n", healthData["status"])
	
	// Show individual health checks
	if checks, ok := healthData["checks"].(map[string]interface{}); ok {
		fmt.Println("\nIndividual Health Checks:")
		for name, checkData := range checks {
			if check, ok := checkData.(map[string]interface{}); ok {
				status := check["status"].(string)
				message := check["message"].(string)
				
				icon := "✅"
				if status == "degraded" {
					icon = "⚠️"
				} else if status == "unhealthy" {
					icon = "❌"
				}
				
				fmt.Printf("%s %s: %s - %s\n", icon, name, status, message)
				
				// Show details if available
				if details, ok := check["details"].(map[string]interface{}); ok {
					for key, value := range details {
						fmt.Printf("    %s: %v\n", key, value)
					}
				}
			}
		}
	}
	
	// Show system info
	if systemInfo, ok := healthData["system_info"].(map[string]interface{}); ok {
		fmt.Println("\nSystem Information:")
		if goVersion, ok := systemInfo["go_version"].(string); ok {
			fmt.Printf("- Go Version: %s\n", goVersion)
		}
		if os, ok := systemInfo["os"].(string); ok {
			fmt.Printf("- OS: %s\n", os)
		}
		if numCPU, ok := systemInfo["num_cpu"].(float64); ok {
			fmt.Printf("- CPU Cores: %.0f\n", numCPU)
		}
		if memoryMB, ok := systemInfo["memory_mb"].(float64); ok {
			fmt.Printf("- Memory Usage: %.0f MB\n", memoryMB)
		}
		if goroutines, ok := systemInfo["num_goroutine"].(float64); ok {
			fmt.Printf("- Goroutines: %.0f\n", goroutines)
		}
	}
}

func testMonitoringDashboard(baseURL string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/dashboard")
	if err != nil {
		fmt.Printf("❌ Dashboard access failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Dashboard Status Code: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	
	if resp.StatusCode == 200 {
		fmt.Println("✅ Dashboard is accessible!")
		fmt.Printf("🌐 Open your browser to: %s/dashboard\n", baseURL)
		
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Dashboard size: %d bytes\n", len(body))
		
		// Check if it contains expected HTML elements
		content := string(body)
		if strings.Contains(content, "KVStore Monitoring") {
			fmt.Println("✅ Dashboard contains monitoring content")
		}
		if strings.Contains(content, "Health Status") {
			fmt.Println("✅ Dashboard contains health status section")
		}
		if strings.Contains(content, "System Statistics") {
			fmt.Println("✅ Dashboard contains system statistics")
		}
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Dashboard not available: %s\n", string(body))
	}
}

