#!/usr/bin/env python3
"""
KVStore Performance Analyzer

This script analyzes KVStore performance metrics, identifies bottlenecks,
and generates optimization recommendations.
"""

import argparse
import json
import logging
import sys
import time
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Tuple

import requests
import yaml
from dataclasses import dataclass
from enum import Enum


class BottleneckType(Enum):
    CPU = "cpu"
    MEMORY = "memory"
    STORAGE = "storage"
    NETWORK = "network"
    RAFT_CONSENSUS = "raft_consensus"
    REQUEST_QUEUE = "request_queue"


@dataclass
class PerformanceMetric:
    name: str
    value: float
    threshold: float
    unit: str
    timestamp: datetime
    severity: str  # low, medium, high, critical


@dataclass
class Bottleneck:
    type: BottleneckType
    component: str
    severity: str
    impact: str
    metrics: List[PerformanceMetric]
    recommendations: List[str]


class PrometheusClient:
    def __init__(self, url: str, timeout: int = 30):
        self.url = url.rstrip('/')
        self.timeout = timeout
        self.session = requests.Session()

    def query(self, query: str, time_param: Optional[datetime] = None) -> Dict:
        """Execute a Prometheus query."""
        params = {'query': query}
        if time_param:
            params['time'] = time_param.timestamp()

        try:
            response = self.session.get(
                f"{self.url}/api/v1/query",
                params=params,
                timeout=self.timeout
            )
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            logging.error(f"Error querying Prometheus: {e}")
            return {}

    def query_range(self, query: str, start: datetime, end: datetime, step: str = "1m") -> Dict:
        """Execute a Prometheus range query."""
        params = {
            'query': query,
            'start': start.timestamp(),
            'end': end.timestamp(),
            'step': step
        }

        try:
            response = self.session.get(
                f"{self.url}/api/v1/query_range",
                params=params,
                timeout=self.timeout
            )
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            logging.error(f"Error querying Prometheus range: {e}")
            return {}


class PerformanceAnalyzer:
    def __init__(self, prometheus_url: str, namespace: str = "kvstore"):
        self.prometheus = PrometheusClient(prometheus_url)
        self.namespace = namespace
        self.bottlenecks = []
        self.metrics = []

    def collect_metrics(self, duration_hours: int = 1) -> None:
        """Collect performance metrics for analysis."""
        end_time = datetime.now()
        start_time = end_time - timedelta(hours=duration_hours)

        # Define critical metrics to collect
        metric_queries = {
            # CPU metrics
            'cpu_usage': f'avg(rate(process_cpu_seconds_total{{job="kvstore", kubernetes_namespace="{self.namespace}"}}[5m])) * 100',
            'cpu_95th': f'quantile(0.95, rate(process_cpu_seconds_total{{job="kvstore", kubernetes_namespace="{self.namespace}"}}[5m])) * 100',
            
            # Memory metrics
            'memory_usage': f'avg(process_resident_memory_bytes{{job="kvstore", kubernetes_namespace="{self.namespace}"}}) / 1024 / 1024 / 1024',
            'memory_95th': f'quantile(0.95, process_resident_memory_bytes{{job="kvstore", kubernetes_namespace="{self.namespace}"}}) / 1024 / 1024 / 1024',
            'heap_usage': f'avg(go_memstats_heap_inuse_bytes{{job="kvstore", kubernetes_namespace="{self.namespace}"}}) / 1024 / 1024 / 1024',
            
            # Request metrics
            'request_rate': f'sum(kvstore:request_rate_5m{{kubernetes_namespace="{self.namespace}"}})',
            'request_latency_95th': f'quantile(0.95, kvstore:latency_95th_5m{{kubernetes_namespace="{self.namespace}"}})',
            'request_latency_99th': f'quantile(0.99, kvstore:latency_95th_5m{{kubernetes_namespace="{self.namespace}"}})',
            'error_rate': f'sum(kvstore:error_rate_5m{{kubernetes_namespace="{self.namespace}"}}) / sum(kvstore:request_rate_5m{{kubernetes_namespace="{self.namespace}"}}) * 100',
            
            # Raft metrics
            'raft_commit_latency': f'avg(kvstore_raft_commit_latency_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'raft_leader_elections': f'sum(increase(kvstore_raft_leader_elections_total{{kubernetes_namespace="{self.namespace}"}}[1h]))',
            'raft_log_replication_lag': f'max(kvstore_raft_log_replication_lag{{kubernetes_namespace="{self.namespace}"}})',
            
            # Storage metrics
            'storage_usage_percent': f'avg((kvstore_storage_used_bytes{{kubernetes_namespace="{self.namespace}"}} / kvstore_storage_total_bytes{{kubernetes_namespace="{self.namespace}"}}) * 100)',
            'storage_iops': f'sum(rate(kvstore_storage_operations_total{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'storage_latency': f'avg(kvstore_storage_operation_duration_seconds{{kubernetes_namespace="{self.namespace}"}})',
            
            # Network metrics
            'network_latency': f'avg(kvstore_network_latency_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'network_throughput': f'sum(rate(kvstore_network_bytes_total{{kubernetes_namespace="{self.namespace}"}}[5m])) / 1024 / 1024',
            
            # Queue metrics
            'request_queue_depth': f'avg(kvstore_queue_depth{{queue_type="request", kubernetes_namespace="{self.namespace}"}})',
            'raft_queue_depth': f'avg(kvstore_queue_depth{{queue_type="raft", kubernetes_namespace="{self.namespace}"}})',
            'storage_queue_depth': f'avg(kvstore_queue_depth{{queue_type="storage", kubernetes_namespace="{self.namespace}"}})',
        }

        # Collect current metrics
        for name, query in metric_queries.items():
            result = self.prometheus.query(query)
            if result.get('status') == 'success' and result.get('data', {}).get('result'):
                value = float(result['data']['result'][0]['value'][1])
                
                # Define thresholds for each metric
                thresholds = {
                    'cpu_usage': 70, 'cpu_95th': 85,
                    'memory_usage': 4, 'memory_95th': 6, 'heap_usage': 3,
                    'request_rate': 1000, 'request_latency_95th': 0.5, 'request_latency_99th': 1.0,
                    'error_rate': 1, 'raft_commit_latency': 0.1, 'raft_leader_elections': 2,
                    'raft_log_replication_lag': 100, 'storage_usage_percent': 80,
                    'storage_iops': 500, 'storage_latency': 0.01, 'network_latency': 0.05,
                    'network_throughput': 100, 'request_queue_depth': 10,
                    'raft_queue_depth': 5, 'storage_queue_depth': 20
                }
                
                units = {
                    'cpu_usage': '%', 'cpu_95th': '%',
                    'memory_usage': 'GB', 'memory_95th': 'GB', 'heap_usage': 'GB',
                    'request_rate': 'req/s', 'request_latency_95th': 's', 'request_latency_99th': 's',
                    'error_rate': '%', 'raft_commit_latency': 's', 'raft_leader_elections': 'count',
                    'raft_log_replication_lag': 'entries', 'storage_usage_percent': '%',
                    'storage_iops': 'ops/s', 'storage_latency': 's', 'network_latency': 's',
                    'network_throughput': 'MB/s', 'request_queue_depth': 'items',
                    'raft_queue_depth': 'items', 'storage_queue_depth': 'items'
                }

                threshold = thresholds.get(name, 0)
                unit = units.get(name, '')
                
                # Determine severity
                ratio = value / threshold if threshold > 0 else 0
                if ratio >= 2:
                    severity = "critical"
                elif ratio >= 1.5:
                    severity = "high"
                elif ratio >= 1:
                    severity = "medium"
                else:
                    severity = "low"

                metric = PerformanceMetric(
                    name=name,
                    value=value,
                    threshold=threshold,
                    unit=unit,
                    timestamp=datetime.now(),
                    severity=severity
                )
                self.metrics.append(metric)

    def identify_bottlenecks(self) -> List[Bottleneck]:
        """Identify performance bottlenecks based on collected metrics."""
        bottlenecks = []

        # Group metrics by component
        cpu_metrics = [m for m in self.metrics if m.name.startswith('cpu')]
        memory_metrics = [m for m in self.metrics if m.name.startswith('memory') or m.name.startswith('heap')]
        request_metrics = [m for m in self.metrics if m.name.startswith('request')]
        raft_metrics = [m for m in self.metrics if m.name.startswith('raft')]
        storage_metrics = [m for m in self.metrics if m.name.startswith('storage')]
        network_metrics = [m for m in self.metrics if m.name.startswith('network')]
        queue_metrics = [m for m in self.metrics if 'queue' in m.name]

        # CPU bottleneck analysis
        high_cpu_metrics = [m for m in cpu_metrics if m.severity in ['high', 'critical']]
        if high_cpu_metrics:
            recommendations = [
                "Consider vertical scaling (increase CPU limits)",
                "Optimize CPU-intensive operations",
                "Review request processing efficiency",
                "Consider horizontal scaling to distribute load"
            ]
            bottlenecks.append(Bottleneck(
                type=BottleneckType.CPU,
                component="CPU",
                severity=max(m.severity for m in high_cpu_metrics),
                impact="High CPU usage may cause increased latency and reduced throughput",
                metrics=high_cpu_metrics,
                recommendations=recommendations
            ))

        # Memory bottleneck analysis
        high_memory_metrics = [m for m in memory_metrics if m.severity in ['high', 'critical']]
        if high_memory_metrics:
            recommendations = [
                "Increase memory limits for KVStore pods",
                "Optimize data structures and caching",
                "Review memory leaks and garbage collection",
                "Consider data compression or archival"
            ]
            bottlenecks.append(Bottleneck(
                type=BottleneckType.MEMORY,
                component="Memory",
                severity=max(m.severity for m in high_memory_metrics),
                impact="High memory usage may cause GC pressure and OOM conditions",
                metrics=high_memory_metrics,
                recommendations=recommendations
            ))

        # Request bottleneck analysis
        high_latency_metrics = [m for m in request_metrics if 'latency' in m.name and m.severity in ['high', 'critical']]
        high_error_metrics = [m for m in request_metrics if 'error' in m.name and m.severity in ['high', 'critical']]
        
        if high_latency_metrics or high_error_metrics:
            recommendations = [
                "Optimize request processing logic",
                "Review database query performance",
                "Implement request caching",
                "Consider load balancing improvements"
            ]
            bottlenecks.append(Bottleneck(
                type=BottleneckType.REQUEST_QUEUE,
                component="Request Processing",
                severity=max(m.severity for m in high_latency_metrics + high_error_metrics),
                impact="High latency and errors impact user experience",
                metrics=high_latency_metrics + high_error_metrics,
                recommendations=recommendations
            ))

        # Raft consensus bottleneck analysis
        high_raft_metrics = [m for m in raft_metrics if m.severity in ['high', 'critical']]
        if high_raft_metrics:
            recommendations = [
                "Optimize network connectivity between nodes",
                "Review Raft configuration (timeouts, batch sizes)",
                "Consider cluster topology changes",
                "Monitor for network partitions"
            ]
            bottlenecks.append(Bottleneck(
                type=BottleneckType.RAFT_CONSENSUS,
                component="Raft Consensus",
                severity=max(m.severity for m in high_raft_metrics),
                impact="Raft issues can cause split-brain scenarios and data inconsistency",
                metrics=high_raft_metrics,
                recommendations=recommendations
            ))

        # Storage bottleneck analysis
        high_storage_metrics = [m for m in storage_metrics if m.severity in ['high', 'critical']]
        if high_storage_metrics:
            recommendations = [
                "Increase storage capacity",
                "Optimize storage I/O patterns",
                "Consider storage class upgrades (e.g., gp3 to io1)",
                "Implement data archival strategies"
            ]
            bottlenecks.append(Bottleneck(
                type=BottleneckType.STORAGE,
                component="Storage",
                severity=max(m.severity for m in high_storage_metrics),
                impact="Storage bottlenecks cause increased latency and potential data loss",
                metrics=high_storage_metrics,
                recommendations=recommendations
            ))

        # Network bottleneck analysis
        high_network_metrics = [m for m in network_metrics if m.severity in ['high', 'critical']]
        if high_network_metrics:
            recommendations = [
                "Optimize network topology",
                "Review network bandwidth allocation",
                "Consider network upgrade or load balancing",
                "Implement data compression"
            ]
            bottlenecks.append(Bottleneck(
                type=BottleneckType.NETWORK,
                component="Network",
                severity=max(m.severity for m in high_network_metrics),
                impact="Network issues affect cluster communication and consensus",
                metrics=high_network_metrics,
                recommendations=recommendations
            ))

        # Queue bottleneck analysis
        high_queue_metrics = [m for m in queue_metrics if m.severity in ['high', 'critical']]
        if high_queue_metrics:
            recommendations = [
                "Increase queue processing workers",
                "Optimize queue processing logic",
                "Implement queue monitoring and alerting",
                "Consider queue partitioning strategies"
            ]
            bottlenecks.append(Bottleneck(
                type=BottleneckType.REQUEST_QUEUE,
                component="Request Queues",
                severity=max(m.severity for m in high_queue_metrics),
                impact="Queue buildup causes increased latency and timeout issues",
                metrics=high_queue_metrics,
                recommendations=recommendations
            ))

        return bottlenecks

    def generate_performance_report(self, output_file: str = None) -> Dict:
        """Generate a comprehensive performance report."""
        report = {
            "timestamp": datetime.now().isoformat(),
            "namespace": self.namespace,
            "analysis_summary": {
                "total_metrics_analyzed": len(self.metrics),
                "bottlenecks_identified": len(self.bottlenecks),
                "critical_issues": len([b for b in self.bottlenecks if b.severity == "critical"]),
                "high_priority_issues": len([b for b in self.bottlenecks if b.severity == "high"])
            },
            "metrics": [
                {
                    "name": m.name,
                    "value": m.value,
                    "threshold": m.threshold,
                    "unit": m.unit,
                    "severity": m.severity,
                    "timestamp": m.timestamp.isoformat()
                } for m in self.metrics
            ],
            "bottlenecks": [
                {
                    "type": b.type.value,
                    "component": b.component,
                    "severity": b.severity,
                    "impact": b.impact,
                    "recommendations": b.recommendations,
                    "affected_metrics": [m.name for m in b.metrics]
                } for b in self.bottlenecks
            ],
            "optimization_recommendations": self._generate_optimization_recommendations()
        }

        if output_file:
            with open(output_file, 'w') as f:
                json.dump(report, f, indent=2)
            logging.info(f"Performance report saved to {output_file}")

        return report

    def _generate_optimization_recommendations(self) -> List[Dict]:
        """Generate system-wide optimization recommendations."""
        recommendations = []

        # Resource optimization
        cpu_metrics = [m for m in self.metrics if m.name.startswith('cpu')]
        memory_metrics = [m for m in self.metrics if m.name.startswith('memory')]
        
        if any(m.severity in ['high', 'critical'] for m in cpu_metrics):
            recommendations.append({
                "category": "Resource Optimization",
                "priority": "High",
                "recommendation": "CPU optimization required",
                "details": [
                    "Increase CPU requests/limits",
                    "Optimize CPU-intensive operations",
                    "Consider horizontal pod autoscaling"
                ]
            })

        if any(m.severity in ['high', 'critical'] for m in memory_metrics):
            recommendations.append({
                "category": "Resource Optimization",
                "priority": "High",
                "recommendation": "Memory optimization required",
                "details": [
                    "Increase memory requests/limits",
                    "Optimize memory usage patterns",
                    "Review garbage collection settings"
                ]
            })

        # Performance optimization
        latency_metrics = [m for m in self.metrics if 'latency' in m.name]
        if any(m.severity in ['high', 'critical'] for m in latency_metrics):
            recommendations.append({
                "category": "Performance Optimization",
                "priority": "High",
                "recommendation": "Latency optimization required",
                "details": [
                    "Optimize request processing paths",
                    "Implement caching strategies",
                    "Review database query performance"
                ]
            })

        # Scaling recommendations
        request_rate_metrics = [m for m in self.metrics if m.name == 'request_rate']
        if request_rate_metrics and request_rate_metrics[0].value > request_rate_metrics[0].threshold * 0.8:
            recommendations.append({
                "category": "Scaling",
                "priority": "Medium",
                "recommendation": "Consider scaling up cluster",
                "details": [
                    "Current load approaching capacity",
                    "Plan for horizontal scaling",
                    "Monitor growth trends"
                ]
            })

        return recommendations

    def run_analysis(self, duration_hours: int = 1, output_file: str = None) -> Dict:
        """Run complete performance analysis."""
        logging.info(f"Starting performance analysis for namespace: {self.namespace}")
        
        # Collect metrics
        self.collect_metrics(duration_hours)
        logging.info(f"Collected {len(self.metrics)} metrics")
        
        # Identify bottlenecks
        self.bottlenecks = self.identify_bottlenecks()
        logging.info(f"Identified {len(self.bottlenecks)} bottlenecks")
        
        # Generate report
        report = self.generate_performance_report(output_file)
        logging.info("Performance analysis completed")
        
        return report


def main():
    parser = argparse.ArgumentParser(description="KVStore Performance Analyzer")
    parser.add_argument("--prometheus-url", required=True,
                       help="Prometheus server URL")
    parser.add_argument("--namespace", default="kvstore",
                       help="Kubernetes namespace (default: kvstore)")
    parser.add_argument("--duration", type=int, default=1,
                       help="Analysis duration in hours (default: 1)")
    parser.add_argument("--output", help="Output file for report (JSON)")
    parser.add_argument("--log-level", default="INFO",
                       choices=["DEBUG", "INFO", "WARNING", "ERROR"],
                       help="Log level")
    
    args = parser.parse_args()
    
    # Configure logging
    logging.basicConfig(
        level=getattr(logging, args.log_level),
        format='%(asctime)s - %(levelname)s - %(message)s'
    )
    
    try:
        analyzer = PerformanceAnalyzer(args.prometheus_url, args.namespace)
        report = analyzer.run_analysis(args.duration, args.output)
        
        # Print summary
        print("\n=== Performance Analysis Summary ===")
        print(f"Namespace: {args.namespace}")
        print(f"Metrics analyzed: {report['analysis_summary']['total_metrics_analyzed']}")
        print(f"Bottlenecks identified: {report['analysis_summary']['bottlenecks_identified']}")
        print(f"Critical issues: {report['analysis_summary']['critical_issues']}")
        print(f"High priority issues: {report['analysis_summary']['high_priority_issues']}")
        
        if report['bottlenecks']:
            print("\n=== Identified Bottlenecks ===")
            for bottleneck in report['bottlenecks']:
                print(f"- {bottleneck['component']} ({bottleneck['severity']}): {bottleneck['impact']}")
        
        if report['optimization_recommendations']:
            print("\n=== Optimization Recommendations ===")
            for rec in report['optimization_recommendations']:
                print(f"- [{rec['priority']}] {rec['recommendation']}")
        
        return 0
        
    except Exception as e:
        logging.error(f"Analysis failed: {e}")
        return 1


if __name__ == "__main__":
    sys.exit(main())