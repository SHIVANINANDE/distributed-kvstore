#!/usr/bin/env python3
"""
KVStore Optimization Advisor

This script analyzes system performance and provides specific optimization
recommendations with implementation details and expected impact.
"""

import argparse
import json
import logging
import sys
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Tuple
from dataclasses import dataclass
from enum import Enum
import requests
import yaml


class OptimizationCategory(Enum):
    RESOURCE_SCALING = "resource_scaling"
    CONFIGURATION = "configuration"
    ARCHITECTURE = "architecture"
    PERFORMANCE = "performance"
    COST_OPTIMIZATION = "cost_optimization"
    SECURITY = "security"


class Priority(Enum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


@dataclass
class OptimizationRecommendation:
    category: OptimizationCategory
    title: str
    description: str
    priority: Priority
    impact: str
    effort: str  # low, medium, high
    implementation_steps: List[str]
    expected_benefits: List[str]
    risks: List[str]
    metrics_to_monitor: List[str]
    estimated_time: str
    cost_impact: str


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


class OptimizationAdvisor:
    def __init__(self, prometheus_url: str, namespace: str = "kvstore"):
        self.prometheus = PrometheusClient(prometheus_url)
        self.namespace = namespace
        self.current_metrics = {}
        self.recommendations = []

    def collect_system_metrics(self) -> Dict:
        """Collect current system metrics for analysis."""
        metric_queries = {
            # Resource utilization
            'cpu_usage_avg': f'avg(rate(process_cpu_seconds_total{{job="kvstore", kubernetes_namespace="{self.namespace}"}}[5m])) * 100',
            'cpu_usage_max': f'max(rate(process_cpu_seconds_total{{job="kvstore", kubernetes_namespace="{self.namespace}"}}[5m])) * 100',
            'memory_usage_avg': f'avg(process_resident_memory_bytes{{job="kvstore", kubernetes_namespace="{self.namespace}"}}) / 1024 / 1024 / 1024',
            'memory_usage_max': f'max(process_resident_memory_bytes{{job="kvstore", kubernetes_namespace="{self.namespace}"}}) / 1024 / 1024 / 1024',
            'storage_usage_percent': f'avg((kvstore_storage_used_bytes{{kubernetes_namespace="{self.namespace}"}} / kvstore_storage_total_bytes{{kubernetes_namespace="{self.namespace}"}}) * 100)',
            
            # Performance metrics
            'request_rate': f'sum(rate(kvstore_requests_total{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'latency_p95': f'histogram_quantile(0.95, rate(kvstore_request_duration_seconds_bucket{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'latency_p99': f'histogram_quantile(0.99, rate(kvstore_request_duration_seconds_bucket{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'error_rate': f'(sum(rate(kvstore_requests_total{{status=~"4..|5..", kubernetes_namespace="{self.namespace}"}}[5m])) / sum(rate(kvstore_requests_total{{kubernetes_namespace="{self.namespace}"}}[5m]))) * 100',
            'throughput': f'sum(rate(kvstore_operations_total{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            
            # Raft consensus metrics
            'raft_commit_latency': f'avg(kvstore_raft_commit_latency_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'raft_leader_elections_rate': f'sum(rate(kvstore_raft_leader_elections_total{{kubernetes_namespace="{self.namespace}"}}[1h]))',
            'raft_log_replication_lag': f'max(kvstore_raft_log_replication_lag{{kubernetes_namespace="{self.namespace}"}})',
            
            # Storage metrics
            'storage_iops': f'sum(rate(kvstore_storage_operations_total{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'storage_latency': f'avg(kvstore_storage_operation_duration_seconds{{kubernetes_namespace="{self.namespace}"}})',
            
            # Network metrics
            'network_latency': f'avg(kvstore_network_latency_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'network_throughput': f'sum(rate(kvstore_network_bytes_total{{kubernetes_namespace="{self.namespace}"}}[5m])) / 1024 / 1024',
            
            # Queue metrics
            'request_queue_depth': f'avg(kvstore_queue_depth{{queue_type="request", kubernetes_namespace="{self.namespace}"}})',
            'raft_queue_depth': f'avg(kvstore_queue_depth{{queue_type="raft", kubernetes_namespace="{self.namespace}"}})',
            
            # Cluster metrics
            'node_count': f'count(up{{job="kvstore", kubernetes_namespace="{self.namespace}"}} == 1)',
            'leader_stability': f'1 - (rate(kvstore_raft_leader_elections_total{{kubernetes_namespace="{self.namespace}"}}[24h]) / 24)',
            
            # Connection metrics
            'active_connections': f'sum(kvstore_active_connections{{kubernetes_namespace="{self.namespace}"}})',
            'connection_utilization': f'(sum(kvstore_active_connections{{kubernetes_namespace="{self.namespace}"}}) / sum(kvstore_max_connections{{kubernetes_namespace="{self.namespace}"}})) * 100',
        }

        for name, query in metric_queries.items():
            result = self.prometheus.query(query)
            if result.get('status') == 'success' and result.get('data', {}).get('result'):
                try:
                    value = float(result['data']['result'][0]['value'][1])
                    self.current_metrics[name] = value
                except (IndexError, ValueError, KeyError):
                    self.current_metrics[name] = 0.0
            else:
                self.current_metrics[name] = 0.0

        logging.info(f"Collected {len(self.current_metrics)} system metrics")
        return self.current_metrics

    def analyze_resource_optimization(self) -> List[OptimizationRecommendation]:
        """Analyze resource utilization and provide optimization recommendations."""
        recommendations = []

        # CPU optimization analysis
        cpu_avg = self.current_metrics.get('cpu_usage_avg', 0)
        cpu_max = self.current_metrics.get('cpu_usage_max', 0)

        if cpu_avg > 80:
            recommendations.append(OptimizationRecommendation(
                category=OptimizationCategory.RESOURCE_SCALING,
                title="Increase CPU Resources",
                description=f"Average CPU usage is {cpu_avg:.1f}%, indicating high resource pressure",
                priority=Priority.HIGH,
                impact="Improved response times and reduced latency",
                effort="low",
                implementation_steps=[
                    "Update Kubernetes resource requests and limits",
                    "kubectl patch deployment kvstore -p '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"kvstore\",\"resources\":{\"requests\":{\"cpu\":\"1000m\"},\"limits\":{\"cpu\":\"2000m\"}}}]}}}}'",
                    "Monitor CPU usage after scaling",
                    "Adjust autoscaling policies if using HPA"
                ],
                expected_benefits=[
                    "Reduced request processing latency",
                    "Better handling of traffic spikes",
                    "Improved overall system stability"
                ],
                risks=[
                    "Increased infrastructure costs",
                    "Potential resource over-provisioning"
                ],
                metrics_to_monitor=[
                    "CPU utilization percentage",
                    "Request latency percentiles",
                    "Pod restart frequency"
                ],
                estimated_time="30 minutes",
                cost_impact="Medium increase in compute costs"
            ))
        elif cpu_avg < 20:
            recommendations.append(OptimizationRecommendation(
                category=OptimizationCategory.COST_OPTIMIZATION,
                title="Reduce CPU Resources",
                description=f"Average CPU usage is {cpu_avg:.1f}%, indicating over-provisioning",
                priority=Priority.MEDIUM,
                impact="Cost savings without performance impact",
                effort="low",
                implementation_steps=[
                    "Gradually reduce CPU requests and limits",
                    "Monitor performance during reduction",
                    "Update autoscaling thresholds",
                    "Document new resource baselines"
                ],
                expected_benefits=[
                    "Reduced infrastructure costs",
                    "Better resource utilization",
                    "More efficient cluster resource allocation"
                ],
                risks=[
                    "Potential performance degradation under load",
                    "Need for careful monitoring during transition"
                ],
                metrics_to_monitor=[
                    "CPU utilization trends",
                    "Response time degradation",
                    "Queue depths and processing delays"
                ],
                estimated_time="1 hour",
                cost_impact="20-40% reduction in compute costs"
            ))

        # Memory optimization analysis
        memory_avg = self.current_metrics.get('memory_usage_avg', 0)
        memory_max = self.current_metrics.get('memory_usage_max', 0)

        if memory_avg > 6:  # Assuming 8GB limit
            recommendations.append(OptimizationRecommendation(
                category=OptimizationCategory.RESOURCE_SCALING,
                title="Optimize Memory Usage",
                description=f"Average memory usage is {memory_avg:.1f}GB, approaching limits",
                priority=Priority.HIGH,
                impact="Prevent OOM kills and improve stability",
                effort="medium",
                implementation_steps=[
                    "Increase memory limits to 12GB",
                    "Optimize application memory usage patterns",
                    "Implement memory profiling and monitoring",
                    "Review garbage collection settings",
                    "Consider memory-efficient data structures"
                ],
                expected_benefits=[
                    "Eliminated OOM-related pod restarts",
                    "Better performance under memory pressure",
                    "Improved garbage collection efficiency"
                ],
                risks=[
                    "Increased memory costs",
                    "Potential masking of memory leaks"
                ],
                metrics_to_monitor=[
                    "Memory usage trends",
                    "GC frequency and duration",
                    "Pod restart frequency"
                ],
                estimated_time="2-4 hours",
                cost_impact="Medium increase in memory costs"
            ))

        # Storage optimization analysis
        storage_usage = self.current_metrics.get('storage_usage_percent', 0)

        if storage_usage > 80:
            recommendations.append(OptimizationRecommendation(
                category=OptimizationCategory.RESOURCE_SCALING,
                title="Expand Storage Capacity",
                description=f"Storage usage is {storage_usage:.1f}%, approaching capacity limits",
                priority=Priority.CRITICAL,
                impact="Prevent storage full conditions and data loss",
                effort="medium",
                implementation_steps=[
                    "Increase PVC size in StatefulSet",
                    "Implement data archival strategy",
                    "Optimize data compression settings",
                    "Set up storage monitoring alerts",
                    "Plan for automated storage expansion"
                ],
                expected_benefits=[
                    "Prevented storage-related failures",
                    "Improved write performance",
                    "Better long-term capacity planning"
                ],
                risks=[
                    "Increased storage costs",
                    "Downtime during storage expansion"
                ],
                metrics_to_monitor=[
                    "Storage utilization percentage",
                    "Storage I/O performance",
                    "Data growth rate"
                ],
                estimated_time="2-6 hours",
                cost_impact="Significant increase in storage costs"
            ))

        return recommendations

    def analyze_performance_optimization(self) -> List[OptimizationRecommendation]:
        """Analyze performance metrics and provide optimization recommendations."""
        recommendations = []

        # Latency optimization
        latency_p95 = self.current_metrics.get('latency_p95', 0)
        latency_p99 = self.current_metrics.get('latency_p99', 0)

        if latency_p95 > 0.5:  # 500ms
            recommendations.append(OptimizationRecommendation(
                category=OptimizationCategory.PERFORMANCE,
                title="Optimize Request Latency",
                description=f"95th percentile latency is {latency_p95*1000:.0f}ms, exceeding target",
                priority=Priority.HIGH,
                impact="Improved user experience and system responsiveness",
                effort="high",
                implementation_steps=[
                    "Profile application to identify bottlenecks",
                    "Optimize database queries and indexes",
                    "Implement request caching layer",
                    "Optimize serialization/deserialization",
                    "Review network topology and routing",
                    "Consider read replicas for load distribution"
                ],
                expected_benefits=[
                    "Reduced user-perceived latency",
                    "Better system throughput",
                    "Improved user satisfaction"
                ],
                risks=[
                    "Complexity in implementation",
                    "Potential consistency trade-offs with caching"
                ],
                metrics_to_monitor=[
                    "Request latency percentiles",
                    "Cache hit rates",
                    "Database query performance"
                ],
                estimated_time="1-2 weeks",
                cost_impact="Minimal cost increase"
            ))

        # Error rate optimization
        error_rate = self.current_metrics.get('error_rate', 0)

        if error_rate > 1:  # 1%
            recommendations.append(OptimizationRecommendation(
                category=OptimizationCategory.PERFORMANCE,
                title="Reduce Error Rate",
                description=f"Error rate is {error_rate:.2f}%, above acceptable threshold",
                priority=Priority.CRITICAL,
                impact="Improved reliability and user experience",
                effort="medium",
                implementation_steps=[
                    "Analyze error logs to identify root causes",
                    "Implement better error handling and retries",
                    "Review timeout configurations",
                    "Improve input validation and sanitization",
                    "Add circuit breaker patterns",
                    "Implement health checks and readiness probes"
                ],
                expected_benefits=[
                    "Reduced user-facing errors",
                    "Better system reliability",
                    "Improved diagnostic capabilities"
                ],
                risks=[
                    "Potential masking of underlying issues",
                    "Increased complexity in error handling"
                ],
                metrics_to_monitor=[
                    "Error rate by endpoint",
                    "Error type distribution",
                    "Recovery time from errors"
                ],
                estimated_time="3-5 days",
                cost_impact="No additional costs"
            ))

        return recommendations

    def analyze_raft_optimization(self) -> List[OptimizationRecommendation]:
        """Analyze Raft consensus metrics and provide optimization recommendations."""
        recommendations = []

        # Raft commit latency
        commit_latency = self.current_metrics.get('raft_commit_latency', 0)

        if commit_latency > 0.1:  # 100ms
            recommendations.append(OptimizationRecommendation(
                category=OptimizationCategory.CONFIGURATION,
                title="Optimize Raft Commit Latency",
                description=f"Raft commit latency is {commit_latency*1000:.0f}ms, affecting consensus performance",
                priority=Priority.HIGH,
                impact="Improved write performance and consistency",
                effort="medium",
                implementation_steps=[
                    "Tune Raft heartbeat interval (reduce from 500ms to 200ms)",
                    "Optimize batch size for log entries",
                    "Review network latency between nodes",
                    "Consider co-locating nodes in same AZ",
                    "Optimize disk I/O for log writes",
                    "Update Raft configuration: heartbeatTimeout: 200ms, electionTimeout: 1s"
                ],
                expected_benefits=[
                    "Faster write acknowledgments",
                    "Reduced write latency",
                    "Better cluster responsiveness"
                ],
                risks=[
                    "Potential increase in leader elections",
                    "Higher network overhead"
                ],
                metrics_to_monitor=[
                    "Raft commit latency",
                    "Leader election frequency",
                    "Network utilization between nodes"
                ],
                estimated_time="2-4 hours",
                cost_impact="No additional costs"
            ))

        # Leader election frequency
        leader_elections = self.current_metrics.get('raft_leader_elections_rate', 0)

        if leader_elections > 0.1:  # More than 1 election per 10 hours
            recommendations.append(OptimizationRecommendation(
                category=OptimizationCategory.CONFIGURATION,
                title="Improve Leader Stability",
                description=f"Leader elections occurring at {leader_elections:.3f}/hour, indicating instability",
                priority=Priority.HIGH,
                impact="Improved cluster stability and consistency",
                effort="medium",
                implementation_steps=[
                    "Investigate network connectivity issues",
                    "Increase leader lease timeout",
                    "Review node resource allocation",
                    "Monitor for GC pauses affecting heartbeats",
                    "Consider dedicated network for cluster communication",
                    "Update configuration: leaderLeaseTimeout: 1s"
                ],
                expected_benefits=[
                    "More stable cluster leadership",
                    "Reduced split-brain scenarios",
                    "Better write consistency"
                ],
                risks=[
                    "Longer recovery time from actual failures",
                    "Potential for longer unavailability windows"
                ],
                metrics_to_monitor=[
                    "Leader election frequency",
                    "Network latency between nodes",
                    "GC pause durations"
                ],
                estimated_time="4-8 hours",
                cost_impact="No additional costs"
            ))

        return recommendations

    def analyze_scaling_optimization(self) -> List[OptimizationRecommendation]:
        """Analyze scaling needs and provide optimization recommendations."""
        recommendations = []

        # Node count analysis
        node_count = self.current_metrics.get('node_count', 0)
        request_rate = self.current_metrics.get('request_rate', 0)

        # Calculate requests per node
        if node_count > 0:
            requests_per_node = request_rate / node_count

            if requests_per_node > 200:  # High load per node
                recommendations.append(OptimizationRecommendation(
                    category=OptimizationCategory.RESOURCE_SCALING,
                    title="Scale Out Cluster",
                    description=f"High load per node ({requests_per_node:.0f} req/s), consider horizontal scaling",
                    priority=Priority.MEDIUM,
                    impact="Improved load distribution and fault tolerance",
                    effort="medium",
                    implementation_steps=[
                        "Increase cluster size from {node_count} to {node_count + 2} nodes",
                        "Update StatefulSet replicas",
                        "Verify Raft cluster configuration",
                        "Monitor load distribution after scaling",
                        "Update monitoring and alerting thresholds"
                    ],
                    expected_benefits=[
                        "Better load distribution",
                        "Improved fault tolerance",
                        "Reduced per-node resource pressure"
                    ],
                    risks=[
                        "Increased infrastructure costs",
                        "More complex cluster management",
                        "Potential for increased consensus overhead"
                    ],
                    metrics_to_monitor=[
                        "Load distribution across nodes",
                        "Raft consensus performance",
                        "Overall cluster latency"
                    ],
                    estimated_time="2-4 hours",
                    cost_impact="Linear increase in infrastructure costs"
                ))

        # Connection utilization analysis
        connection_util = self.current_metrics.get('connection_utilization', 0)

        if connection_util > 80:
            recommendations.append(OptimizationRecommendation(
                category=OptimizationCategory.CONFIGURATION,
                title="Optimize Connection Pool",
                description=f"Connection utilization is {connection_util:.1f}%, approaching limits",
                priority=Priority.MEDIUM,
                impact="Improved connection handling and reduced connection errors",
                effort="low",
                implementation_steps=[
                    "Increase maximum connection pool size",
                    "Optimize connection timeout settings",
                    "Implement connection pooling optimization",
                    "Review client connection patterns",
                    "Consider connection multiplexing"
                ],
                expected_benefits=[
                    "Reduced connection rejection errors",
                    "Better client connection handling",
                    "Improved scalability"
                ],
                risks=[
                    "Increased memory usage per connection",
                    "Potential resource exhaustion"
                ],
                metrics_to_monitor=[
                    "Connection utilization percentage",
                    "Connection error rates",
                    "Memory usage per connection"
                ],
                estimated_time="1-2 hours",
                cost_impact="Minimal memory cost increase"
            ))

        return recommendations

    def generate_optimization_report(self, output_file: str = None) -> Dict:
        """Generate comprehensive optimization report."""
        
        # Prioritize recommendations
        self.recommendations.sort(key=lambda x: (
            ['low', 'medium', 'high', 'critical'].index(x.priority.value),
            ['low', 'medium', 'high'].index(x.effort)
        ), reverse=True)

        report = {
            "timestamp": datetime.now().isoformat(),
            "namespace": self.namespace,
            "current_metrics": self.current_metrics,
            "optimization_summary": {
                "total_recommendations": len(self.recommendations),
                "critical_priority": len([r for r in self.recommendations if r.priority == Priority.CRITICAL]),
                "high_priority": len([r for r in self.recommendations if r.priority == Priority.HIGH]),
                "medium_priority": len([r for r in self.recommendations if r.priority == Priority.MEDIUM]),
                "low_priority": len([r for r in self.recommendations if r.priority == Priority.LOW]),
                "by_category": {
                    category.value: len([r for r in self.recommendations if r.category == category])
                    for category in OptimizationCategory
                }
            },
            "recommendations": [
                {
                    "category": rec.category.value,
                    "title": rec.title,
                    "description": rec.description,
                    "priority": rec.priority.value,
                    "impact": rec.impact,
                    "effort": rec.effort,
                    "implementation_steps": rec.implementation_steps,
                    "expected_benefits": rec.expected_benefits,
                    "risks": rec.risks,
                    "metrics_to_monitor": rec.metrics_to_monitor,
                    "estimated_time": rec.estimated_time,
                    "cost_impact": rec.cost_impact
                } for rec in self.recommendations
            ],
            "quick_wins": [
                rec for rec in self.recommendations 
                if rec.effort == "low" and rec.priority in [Priority.HIGH, Priority.CRITICAL]
            ],
            "implementation_roadmap": self._generate_implementation_roadmap()
        }

        if output_file:
            with open(output_file, 'w') as f:
                json.dump(report, f, indent=2, default=str)
            logging.info(f"Optimization report saved to {output_file}")

        return report

    def _generate_implementation_roadmap(self) -> Dict:
        """Generate a prioritized implementation roadmap."""
        roadmap = {
            "immediate": [],  # Critical priority, low effort
            "short_term": [],  # High priority or medium effort
            "medium_term": [],  # Medium priority or high effort
            "long_term": []  # Low priority or complex implementations
        }

        for rec in self.recommendations:
            if rec.priority == Priority.CRITICAL and rec.effort == "low":
                roadmap["immediate"].append(rec.title)
            elif rec.priority in [Priority.CRITICAL, Priority.HIGH] and rec.effort in ["low", "medium"]:
                roadmap["short_term"].append(rec.title)
            elif rec.priority == Priority.MEDIUM or rec.effort == "high":
                roadmap["medium_term"].append(rec.title)
            else:
                roadmap["long_term"].append(rec.title)

        return roadmap

    def run_optimization_analysis(self, output_file: str = None) -> Dict:
        """Run complete optimization analysis."""
        logging.info("Starting optimization analysis")

        # Collect system metrics
        self.collect_system_metrics()

        # Run optimization analyses
        resource_recs = self.analyze_resource_optimization()
        performance_recs = self.analyze_performance_optimization()
        raft_recs = self.analyze_raft_optimization()
        scaling_recs = self.analyze_scaling_optimization()

        # Combine all recommendations
        self.recommendations = resource_recs + performance_recs + raft_recs + scaling_recs

        logging.info(f"Generated {len(self.recommendations)} optimization recommendations")

        # Generate report
        report = self.generate_optimization_report(output_file)

        return report


def main():
    parser = argparse.ArgumentParser(description="KVStore Optimization Advisor")
    parser.add_argument("--prometheus-url", required=True,
                       help="Prometheus server URL")
    parser.add_argument("--namespace", default="kvstore",
                       help="Kubernetes namespace (default: kvstore)")
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
        advisor = OptimizationAdvisor(args.prometheus_url, args.namespace)
        report = advisor.run_optimization_analysis(args.output)

        # Print summary
        print("\n=== Optimization Analysis Summary ===")
        print(f"Namespace: {args.namespace}")
        print(f"Total Recommendations: {report['optimization_summary']['total_recommendations']}")
        print(f"Critical: {report['optimization_summary']['critical_priority']}")
        print(f"High: {report['optimization_summary']['high_priority']}")
        print(f"Medium: {report['optimization_summary']['medium_priority']}")
        print(f"Low: {report['optimization_summary']['low_priority']}")

        if report['recommendations']:
            print("\n=== Top Priority Recommendations ===")
            for i, rec in enumerate(report['recommendations'][:5], 1):
                print(f"{i}. {rec['title']} ({rec['priority']} priority, {rec['effort']} effort)")
                print(f"   Impact: {rec['impact']}")

        print(f"\n=== Implementation Roadmap ===")
        roadmap = report['implementation_roadmap']
        if roadmap['immediate']:
            print(f"Immediate: {', '.join(roadmap['immediate'])}")
        if roadmap['short_term']:
            print(f"Short-term: {', '.join(roadmap['short_term'])}")
        if roadmap['medium_term']:
            print(f"Medium-term: {', '.join(roadmap['medium_term'])}")
        if roadmap['long_term']:
            print(f"Long-term: {', '.join(roadmap['long_term'])}")

        return 0

    except Exception as e:
        logging.error(f"Optimization analysis failed: {e}")
        return 1


if __name__ == "__main__":
    sys.exit(main())