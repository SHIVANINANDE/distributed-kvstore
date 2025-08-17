#!/usr/bin/env python3
"""
KVStore Performance Regression Detector

This script detects performance regressions by comparing current metrics
with historical baselines and statistical models.
"""

import argparse
import json
import logging
import numpy as np
import pandas as pd
import sys
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Tuple
from dataclasses import dataclass
from enum import Enum
import requests
from scipy import stats
from sklearn.ensemble import IsolationForest
from sklearn.preprocessing import StandardScaler


class RegressionSeverity(Enum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


@dataclass
class RegressionAlert:
    metric_name: str
    current_value: float
    baseline_value: float
    deviation_percent: float
    severity: RegressionSeverity
    confidence: float
    timestamp: datetime
    description: str
    recommendations: List[str]


class PrometheusClient:
    def __init__(self, url: str, timeout: int = 30):
        self.url = url.rstrip('/')
        self.timeout = timeout
        self.session = requests.Session()

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
            logging.error(f"Error querying Prometheus: {e}")
            return {}


class RegressionDetector:
    def __init__(self, prometheus_url: str, namespace: str = "kvstore"):
        self.prometheus = PrometheusClient(prometheus_url)
        self.namespace = namespace
        self.regression_alerts = []
        
        # Statistical thresholds
        self.deviation_thresholds = {
            'low': 10.0,      # 10% deviation
            'medium': 25.0,   # 25% deviation
            'high': 50.0,     # 50% deviation
            'critical': 100.0 # 100% deviation
        }
        
        # Confidence thresholds
        self.confidence_threshold = 0.8

    def collect_baseline_data(self, days_back: int = 7) -> Dict[str, pd.DataFrame]:
        """Collect baseline performance data for comparison."""
        end_time = datetime.now() - timedelta(days=1)  # Exclude today
        start_time = end_time - timedelta(days=days_back)
        
        # Define key performance metrics
        baseline_metrics = {
            'request_latency_p95': f'histogram_quantile(0.95, rate(kvstore_request_duration_seconds_bucket{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'request_latency_p99': f'histogram_quantile(0.99, rate(kvstore_request_duration_seconds_bucket{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'request_rate': f'sum(rate(kvstore_requests_total{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'error_rate': f'sum(rate(kvstore_requests_total{{status=~"4..|5..", kubernetes_namespace="{self.namespace}"}}[5m])) / sum(rate(kvstore_requests_total{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'cpu_usage': f'avg(rate(process_cpu_seconds_total{{job="kvstore", kubernetes_namespace="{self.namespace}"}}[5m])) * 100',
            'memory_usage': f'avg(process_resident_memory_bytes{{job="kvstore", kubernetes_namespace="{self.namespace}"}}) / 1024 / 1024 / 1024',
            'raft_commit_latency': f'avg(kvstore_raft_commit_latency_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'storage_latency': f'avg(kvstore_storage_operation_duration_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'network_latency': f'avg(kvstore_network_latency_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'throughput': f'sum(rate(kvstore_operations_total{{kubernetes_namespace="{self.namespace}"}}[5m]))'
        }
        
        baseline_data = {}
        
        for metric_name, query in baseline_metrics.items():
            logging.info(f"Collecting baseline data for {metric_name}")
            result = self.prometheus.query_range(query, start_time, end_time, "5m")
            
            if result.get('status') == 'success' and result.get('data', {}).get('result'):
                # Convert to pandas DataFrame for easier analysis
                df_list = []
                for series in result['data']['result']:
                    values = series['values']
                    df = pd.DataFrame(values, columns=['timestamp', 'value'])
                    df['timestamp'] = pd.to_datetime(df['timestamp'], unit='s')
                    df['value'] = pd.to_numeric(df['value'], errors='coerce')
                    df = df.dropna()
                    df_list.append(df)
                
                if df_list:
                    # Combine all series (if multiple pods)
                    combined_df = pd.concat(df_list, ignore_index=True)
                    combined_df = combined_df.groupby('timestamp')['value'].mean().reset_index()
                    baseline_data[metric_name] = combined_df
                    logging.info(f"Collected {len(combined_df)} baseline points for {metric_name}")
        
        return baseline_data

    def collect_current_data(self, hours_back: int = 1) -> Dict[str, pd.DataFrame]:
        """Collect current performance data for comparison."""
        end_time = datetime.now()
        start_time = end_time - timedelta(hours=hours_back)
        
        # Use same metrics as baseline
        current_metrics = {
            'request_latency_p95': f'histogram_quantile(0.95, rate(kvstore_request_duration_seconds_bucket{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'request_latency_p99': f'histogram_quantile(0.99, rate(kvstore_request_duration_seconds_bucket{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'request_rate': f'sum(rate(kvstore_requests_total{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'error_rate': f'sum(rate(kvstore_requests_total{{status=~"4..|5..", kubernetes_namespace="{self.namespace}"}}[5m])) / sum(rate(kvstore_requests_total{{kubernetes_namespace="{self.namespace}"}}[5m]))',
            'cpu_usage': f'avg(rate(process_cpu_seconds_total{{job="kvstore", kubernetes_namespace="{self.namespace}"}}[5m])) * 100',
            'memory_usage': f'avg(process_resident_memory_bytes{{job="kvstore", kubernetes_namespace="{self.namespace}"}}) / 1024 / 1024 / 1024',
            'raft_commit_latency': f'avg(kvstore_raft_commit_latency_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'storage_latency': f'avg(kvstore_storage_operation_duration_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'network_latency': f'avg(kvstore_network_latency_seconds{{kubernetes_namespace="{self.namespace}"}})',
            'throughput': f'sum(rate(kvstore_operations_total{{kubernetes_namespace="{self.namespace}"}}[5m]))'
        }
        
        current_data = {}
        
        for metric_name, query in current_metrics.items():
            result = self.prometheus.query_range(query, start_time, end_time, "1m")
            
            if result.get('status') == 'success' and result.get('data', {}).get('result'):
                df_list = []
                for series in result['data']['result']:
                    values = series['values']
                    df = pd.DataFrame(values, columns=['timestamp', 'value'])
                    df['timestamp'] = pd.to_datetime(df['timestamp'], unit='s')
                    df['value'] = pd.to_numeric(df['value'], errors='coerce')
                    df = df.dropna()
                    df_list.append(df)
                
                if df_list:
                    combined_df = pd.concat(df_list, ignore_index=True)
                    combined_df = combined_df.groupby('timestamp')['value'].mean().reset_index()
                    current_data[metric_name] = combined_df
        
        return current_data

    def detect_statistical_regressions(self, baseline_data: Dict[str, pd.DataFrame], 
                                     current_data: Dict[str, pd.DataFrame]) -> List[RegressionAlert]:
        """Detect regressions using statistical analysis."""
        alerts = []
        
        for metric_name in baseline_data.keys():
            if metric_name not in current_data:
                continue
                
            baseline_values = baseline_data[metric_name]['value'].values
            current_values = current_data[metric_name]['value'].values
            
            if len(baseline_values) < 10 or len(current_values) < 5:
                continue  # Not enough data for reliable analysis
            
            # Calculate baseline statistics
            baseline_mean = np.mean(baseline_values)
            baseline_std = np.std(baseline_values)
            current_mean = np.mean(current_values)
            
            # Skip if baseline is zero (to avoid division by zero)
            if baseline_mean == 0:
                continue
            
            # Calculate deviation percentage
            deviation_percent = abs((current_mean - baseline_mean) / baseline_mean) * 100
            
            # Perform statistical tests
            # 1. T-test for mean difference
            t_stat, p_value = stats.ttest_ind(baseline_values, current_values)
            
            # 2. Mann-Whitney U test (non-parametric)
            u_stat, u_p_value = stats.mannwhitneyu(baseline_values, current_values, alternative='two-sided')
            
            # 3. Kolmogorov-Smirnov test for distribution difference
            ks_stat, ks_p_value = stats.ks_2samp(baseline_values, current_values)
            
            # Calculate confidence based on statistical significance
            confidence = 1 - min(p_value, u_p_value, ks_p_value)
            
            # Determine if regression is detected
            is_regression = (
                confidence > self.confidence_threshold and
                deviation_percent > self.deviation_thresholds['low']
            )
            
            if is_regression:
                # Determine severity based on deviation
                if deviation_percent >= self.deviation_thresholds['critical']:
                    severity = RegressionSeverity.CRITICAL
                elif deviation_percent >= self.deviation_thresholds['high']:
                    severity = RegressionSeverity.HIGH
                elif deviation_percent >= self.deviation_thresholds['medium']:
                    severity = RegressionSeverity.MEDIUM
                else:
                    severity = RegressionSeverity.LOW
                
                # Generate description and recommendations
                direction = "increased" if current_mean > baseline_mean else "decreased"
                description = f"{metric_name} has {direction} by {deviation_percent:.1f}% compared to baseline"
                
                recommendations = self._generate_metric_recommendations(metric_name, direction, severity)
                
                alert = RegressionAlert(
                    metric_name=metric_name,
                    current_value=current_mean,
                    baseline_value=baseline_mean,
                    deviation_percent=deviation_percent,
                    severity=severity,
                    confidence=confidence,
                    timestamp=datetime.now(),
                    description=description,
                    recommendations=recommendations
                )
                alerts.append(alert)
                
                logging.warning(f"Regression detected: {description} (confidence: {confidence:.3f})")
        
        return alerts

    def detect_anomalies(self, baseline_data: Dict[str, pd.DataFrame], 
                        current_data: Dict[str, pd.DataFrame]) -> List[RegressionAlert]:
        """Detect anomalies using machine learning techniques."""
        alerts = []
        
        for metric_name in baseline_data.keys():
            if metric_name not in current_data:
                continue
                
            baseline_values = baseline_data[metric_name]['value'].values
            current_values = current_data[metric_name]['value'].values
            
            if len(baseline_values) < 50:  # Need more data for ML
                continue
            
            # Prepare data for anomaly detection
            all_values = np.concatenate([baseline_values, current_values])
            
            # Normalize the data
            scaler = StandardScaler()
            normalized_values = scaler.fit_transform(all_values.reshape(-1, 1))
            
            # Train Isolation Forest on baseline data
            baseline_normalized = normalized_values[:len(baseline_values)]
            current_normalized = normalized_values[len(baseline_values):]
            
            iso_forest = IsolationForest(contamination=0.1, random_state=42)
            iso_forest.fit(baseline_normalized)
            
            # Predict anomalies in current data
            anomaly_scores = iso_forest.decision_function(current_normalized)
            anomaly_predictions = iso_forest.predict(current_normalized)
            
            # Calculate percentage of anomalous points
            anomaly_percentage = (anomaly_predictions == -1).mean() * 100
            
            if anomaly_percentage > 20:  # More than 20% anomalous points
                current_mean = np.mean(current_values)
                baseline_mean = np.mean(baseline_values)
                deviation_percent = abs((current_mean - baseline_mean) / baseline_mean) * 100 if baseline_mean != 0 else 0
                
                # Determine severity based on anomaly percentage
                if anomaly_percentage >= 80:
                    severity = RegressionSeverity.CRITICAL
                elif anomaly_percentage >= 60:
                    severity = RegressionSeverity.HIGH
                elif anomaly_percentage >= 40:
                    severity = RegressionSeverity.MEDIUM
                else:
                    severity = RegressionSeverity.LOW
                
                direction = "increased" if current_mean > baseline_mean else "decreased"
                description = f"{metric_name} shows anomalous behavior ({anomaly_percentage:.1f}% of points are anomalous)"
                recommendations = self._generate_metric_recommendations(metric_name, direction, severity)
                
                alert = RegressionAlert(
                    metric_name=metric_name,
                    current_value=current_mean,
                    baseline_value=baseline_mean,
                    deviation_percent=deviation_percent,
                    severity=severity,
                    confidence=min(anomaly_percentage / 100, 1.0),
                    timestamp=datetime.now(),
                    description=description,
                    recommendations=recommendations
                )
                alerts.append(alert)
        
        return alerts

    def _generate_metric_recommendations(self, metric_name: str, direction: str, 
                                       severity: RegressionSeverity) -> List[str]:
        """Generate specific recommendations based on metric and regression type."""
        recommendations = []
        
        if "latency" in metric_name:
            if direction == "increased":
                recommendations.extend([
                    "Investigate recent code changes that might affect performance",
                    "Check for increased load or traffic patterns",
                    "Review database query performance",
                    "Monitor resource utilization (CPU, memory, storage)",
                    "Consider scaling up resources or optimizing bottlenecks"
                ])
            else:
                recommendations.append("Monitor to ensure latency improvement is sustainable")
        
        elif "error_rate" in metric_name:
            if direction == "increased":
                recommendations.extend([
                    "Investigate error logs for root cause analysis",
                    "Check for recent deployments or configuration changes",
                    "Verify service health and dependencies",
                    "Review circuit breaker and retry policies",
                    "Consider rolling back recent changes if necessary"
                ])
        
        elif "cpu_usage" in metric_name:
            if direction == "increased":
                recommendations.extend([
                    "Analyze CPU profiling data to identify hotspots",
                    "Check for CPU-intensive operations or inefficient algorithms",
                    "Consider vertical scaling (increase CPU limits)",
                    "Review concurrent operations and optimize if needed"
                ])
        
        elif "memory_usage" in metric_name:
            if direction == "increased":
                recommendations.extend([
                    "Investigate memory leaks or inefficient memory usage",
                    "Review object lifecycle and garbage collection",
                    "Consider increasing memory limits",
                    "Optimize data structures and caching strategies"
                ])
        
        elif "raft_commit_latency" in metric_name:
            if direction == "increased":
                recommendations.extend([
                    "Check network connectivity between cluster nodes",
                    "Review Raft configuration parameters",
                    "Monitor for network partitions or high latency",
                    "Consider cluster topology optimization"
                ])
        
        elif "throughput" in metric_name:
            if direction == "decreased":
                recommendations.extend([
                    "Investigate bottlenecks in request processing pipeline",
                    "Check for resource constraints (CPU, memory, storage)",
                    "Review concurrent processing capabilities",
                    "Consider horizontal scaling or optimization"
                ])
        
        # Add severity-specific recommendations
        if severity == RegressionSeverity.CRITICAL:
            recommendations.insert(0, "CRITICAL: Immediate investigation required")
            recommendations.append("Consider emergency rollback if recent deployment")
        elif severity == RegressionSeverity.HIGH:
            recommendations.insert(0, "HIGH PRIORITY: Schedule immediate investigation")
        
        return recommendations

    def generate_regression_report(self, output_file: str = None) -> Dict:
        """Generate comprehensive regression analysis report."""
        report = {
            "timestamp": datetime.now().isoformat(),
            "namespace": self.namespace,
            "analysis_summary": {
                "total_alerts": len(self.regression_alerts),
                "critical_alerts": len([a for a in self.regression_alerts if a.severity == RegressionSeverity.CRITICAL]),
                "high_alerts": len([a for a in self.regression_alerts if a.severity == RegressionSeverity.HIGH]),
                "medium_alerts": len([a for a in self.regression_alerts if a.severity == RegressionSeverity.MEDIUM]),
                "low_alerts": len([a for a in self.regression_alerts if a.severity == RegressionSeverity.LOW])
            },
            "regression_alerts": [
                {
                    "metric_name": alert.metric_name,
                    "current_value": alert.current_value,
                    "baseline_value": alert.baseline_value,
                    "deviation_percent": alert.deviation_percent,
                    "severity": alert.severity.value,
                    "confidence": alert.confidence,
                    "timestamp": alert.timestamp.isoformat(),
                    "description": alert.description,
                    "recommendations": alert.recommendations
                } for alert in self.regression_alerts
            ],
            "overall_health_score": self._calculate_health_score()
        }
        
        if output_file:
            with open(output_file, 'w') as f:
                json.dump(report, f, indent=2)
            logging.info(f"Regression report saved to {output_file}")
        
        return report

    def _calculate_health_score(self) -> float:
        """Calculate overall system health score based on regressions."""
        if not self.regression_alerts:
            return 100.0
        
        # Weight different severities
        severity_weights = {
            RegressionSeverity.CRITICAL: 50,
            RegressionSeverity.HIGH: 25,
            RegressionSeverity.MEDIUM: 10,
            RegressionSeverity.LOW: 5
        }
        
        total_penalty = sum(severity_weights.get(alert.severity, 0) for alert in self.regression_alerts)
        health_score = max(0, 100 - total_penalty)
        
        return health_score

    def run_regression_detection(self, baseline_days: int = 7, current_hours: int = 1, 
                               output_file: str = None) -> Dict:
        """Run complete regression detection analysis."""
        logging.info("Starting regression detection analysis")
        
        # Collect baseline data
        logging.info(f"Collecting baseline data for {baseline_days} days")
        baseline_data = self.collect_baseline_data(baseline_days)
        
        # Collect current data
        logging.info(f"Collecting current data for {current_hours} hours")
        current_data = self.collect_current_data(current_hours)
        
        # Detect statistical regressions
        logging.info("Running statistical regression analysis")
        statistical_alerts = self.detect_statistical_regressions(baseline_data, current_data)
        
        # Detect anomalies using ML
        logging.info("Running anomaly detection analysis")
        anomaly_alerts = self.detect_anomalies(baseline_data, current_data)
        
        # Combine and deduplicate alerts
        self.regression_alerts = statistical_alerts + anomaly_alerts
        
        # Remove duplicates (same metric with lower confidence)
        unique_alerts = {}
        for alert in self.regression_alerts:
            if alert.metric_name not in unique_alerts or alert.confidence > unique_alerts[alert.metric_name].confidence:
                unique_alerts[alert.metric_name] = alert
        
        self.regression_alerts = list(unique_alerts.values())
        
        logging.info(f"Detected {len(self.regression_alerts)} regressions")
        
        # Generate report
        report = self.generate_regression_report(output_file)
        
        return report


def main():
    parser = argparse.ArgumentParser(description="KVStore Performance Regression Detector")
    parser.add_argument("--prometheus-url", required=True,
                       help="Prometheus server URL")
    parser.add_argument("--namespace", default="kvstore",
                       help="Kubernetes namespace (default: kvstore)")
    parser.add_argument("--baseline-days", type=int, default=7,
                       help="Days of baseline data to collect (default: 7)")
    parser.add_argument("--current-hours", type=int, default=1,
                       help="Hours of current data to analyze (default: 1)")
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
        detector = RegressionDetector(args.prometheus_url, args.namespace)
        report = detector.run_regression_detection(
            args.baseline_days, args.current_hours, args.output
        )
        
        # Print summary
        print("\n=== Regression Detection Summary ===")
        print(f"Namespace: {args.namespace}")
        print(f"Health Score: {report['overall_health_score']:.1f}/100")
        print(f"Total Alerts: {report['analysis_summary']['total_alerts']}")
        print(f"Critical: {report['analysis_summary']['critical_alerts']}")
        print(f"High: {report['analysis_summary']['high_alerts']}")
        print(f"Medium: {report['analysis_summary']['medium_alerts']}")
        print(f"Low: {report['analysis_summary']['low_alerts']}")
        
        if report['regression_alerts']:
            print("\n=== Detected Regressions ===")
            for alert in report['regression_alerts']:
                print(f"- {alert['metric_name']} ({alert['severity']}): {alert['description']}")
                print(f"  Deviation: {alert['deviation_percent']:.1f}% (confidence: {alert['confidence']:.3f})")
        
        return 0 if report['analysis_summary']['critical_alerts'] == 0 else 1
        
    except Exception as e:
        logging.error(f"Regression detection failed: {e}")
        return 1


if __name__ == "__main__":
    sys.exit(main())