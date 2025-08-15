package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"distributed-kvstore/internal/logging"
)

func main() {
	fmt.Println("=== Correlation ID Demo ===")
	fmt.Println("This demo shows how correlation IDs work across HTTP requests")
	fmt.Println()

	// Wait a moment for any previous server to shut down
	time.Sleep(1 * time.Second)

	baseURL := "http://localhost:8080"

	// Test 1: Request without correlation ID (middleware generates one)
	fmt.Println("1. Making request without correlation ID:")
	testRequest(baseURL+"/health", "", "")

	// Test 2: Request with custom correlation ID
	fmt.Println("\n2. Making request with custom correlation ID:")
	customCorrelationID := logging.GenerateCorrelationID()
	customRequestID := logging.GenerateRequestID()
	testRequest(baseURL+"/health", customCorrelationID, customRequestID)

	// Test 3: Multiple requests with same correlation ID (simulating distributed request)
	fmt.Println("\n3. Making multiple requests with same correlation ID:")
	sharedCorrelationID := logging.GenerateCorrelationID()
	for i := 1; i <= 3; i++ {
		requestID := logging.GenerateRequestID()
		fmt.Printf("Request %d with correlation ID %s:\n", i, sharedCorrelationID)
		testRequest(baseURL+"/health", sharedCorrelationID, requestID)
		if i < 3 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	fmt.Println("\n4. Making a key-value operation request:")
	testKVOperation(baseURL)
}

func testRequest(url, correlationID, requestID string) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	// Add correlation ID headers if provided
	if correlationID != "" {
		req.Header.Set("X-Correlation-ID", correlationID)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	// Print the correlation ID returned by server
	serverCorrelationID := resp.Header.Get("X-Correlation-ID")
	serverRequestID := resp.Header.Get("X-Request-ID")

	fmt.Printf("  Status: %d\n", resp.StatusCode)
	fmt.Printf("  Server Correlation ID: %s\n", serverCorrelationID)
	fmt.Printf("  Server Request ID: %s\n", serverRequestID)

	if correlationID != "" && serverCorrelationID != correlationID {
		fmt.Printf("  ⚠️  Correlation ID mismatch! Sent: %s, Received: %s\n", correlationID, serverCorrelationID)
	} else if correlationID != "" {
		fmt.Printf("  ✅ Correlation ID preserved correctly\n")
	} else {
		fmt.Printf("  ✅ Server generated correlation ID\n")
	}

	// Pretty print JSON response if it's JSON
	var jsonResponse map[string]interface{}
	if err := json.Unmarshal(body, &jsonResponse); err == nil {
		prettyJSON, _ := json.MarshalIndent(jsonResponse, "  ", "  ")
		fmt.Printf("  Response: %s\n", string(prettyJSON))
	} else {
		fmt.Printf("  Response: %s\n", string(body))
	}
}

func testKVOperation(baseURL string) {
	correlationID := logging.GenerateCorrelationID()
	
	// First, put a key
	putData := map[string]interface{}{
		"value": "test-value-with-correlation",
	}
	jsonData, _ := json.Marshal(putData)

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("PUT", baseURL+"/api/v1/kv/demo-key", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error creating PUT request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", correlationID)

	fmt.Printf("  PUT request with correlation ID %s:\n", correlationID)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  Error making PUT request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("    PUT Status: %d\n", resp.StatusCode)
	fmt.Printf("    PUT Correlation ID: %s\n", resp.Header.Get("X-Correlation-ID"))

	// Now get the key with same correlation ID
	time.Sleep(100 * time.Millisecond)
	
	getReq, err := http.NewRequest("GET", baseURL+"/api/v1/kv/demo-key", nil)
	if err != nil {
		fmt.Printf("Error creating GET request: %v\n", err)
		return
	}
	
	getReq.Header.Set("X-Correlation-ID", correlationID)

	fmt.Printf("  GET request with same correlation ID:\n")
	getResp, err := client.Do(getReq)
	if err != nil {
		fmt.Printf("  Error making GET request: %v\n", err)
		return
	}
	defer getResp.Body.Close()

	getBody, _ := io.ReadAll(getResp.Body)
	fmt.Printf("    GET Status: %d\n", getResp.StatusCode)
	fmt.Printf("    GET Correlation ID: %s\n", getResp.Header.Get("X-Correlation-ID"))
	fmt.Printf("    GET Response: %s\n", string(getBody))
	
	if getResp.Header.Get("X-Correlation-ID") == correlationID {
		fmt.Printf("    ✅ Correlation ID preserved across PUT and GET operations\n")
	} else {
		fmt.Printf("    ⚠️  Correlation ID not preserved\n")
	}
}