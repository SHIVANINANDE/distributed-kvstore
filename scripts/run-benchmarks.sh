#!/bin/bash

# Performance Benchmark Runner
# This script runs actual performance benchmarks and generates metrics

echo "🚀 Running Distributed KV Store Performance Benchmarks"
echo "======================================================"

cd "$(dirname "$0")"

# Create results directory
mkdir -p benchmark_results
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
RESULTS_FILE="benchmark_results/performance_${TIMESTAMP}.txt"

echo "📊 Benchmark Results - $(date)" > $RESULTS_FILE
echo "======================================" >> $RESULTS_FILE
echo "" >> $RESULTS_FILE

# Run Go benchmarks
echo "Running Go benchmarks..."
echo "🔧 Go Benchmark Results:" >> $RESULTS_FILE
go test -bench=BenchmarkRealPerformance -benchmem ./benchmarks/ -timeout=10m >> $RESULTS_FILE 2>&1

echo "" >> $RESULTS_FILE

# Run performance tests
echo "Running performance tests..."
echo "📈 Performance Test Results:" >> $RESULTS_FILE
go test -v -run="TestReal" ./benchmarks/ -timeout=10m >> $RESULTS_FILE 2>&1

echo "" >> $RESULTS_FILE

# System info
echo "💻 System Information:" >> $RESULTS_FILE
echo "OS: $(uname -s)" >> $RESULTS_FILE
echo "Arch: $(uname -m)" >> $RESULTS_FILE
echo "CPU Cores: $(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 'Unknown')" >> $RESULTS_FILE
echo "Go Version: $(go version)" >> $RESULTS_FILE
echo "Timestamp: $(date)" >> $RESULTS_FILE

echo ""
echo "✅ Benchmark completed! Results saved to: $RESULTS_FILE"
echo ""
echo "📋 Quick Summary:"
echo "==================="

# Extract key metrics for quick view
if [[ -f $RESULTS_FILE ]]; then
    echo "Performance highlights:"
    grep -E "(ops/sec|ms/op|PUT Performance|GET Performance|Mixed Workload)" $RESULTS_FILE | head -10
    echo ""
    echo "Full results in: $RESULTS_FILE"
fi

echo ""
echo "🎯 Use these verified metrics in your resume!"
