# Variables for KVStore Application Module

variable "namespace" {
  description = "Kubernetes namespace for KVStore"
  type        = string
  default     = "kvstore"
}

variable "app_version" {
  description = "Version of the KVStore application"
  type        = string
}

variable "image_repository" {
  description = "Container image repository"
  type        = string
  default     = "ghcr.io/kvstore/kvstore"
}

variable "environment" {
  description = "Environment name (dev, staging, prod)"
  type        = string
}

variable "cluster_size" {
  description = "Number of KVStore nodes in the cluster"
  type        = number
  default     = 3
  
  validation {
    condition     = var.cluster_size >= 1 && var.cluster_size <= 10
    error_message = "Cluster size must be between 1 and 10."
  }
}

variable "replication_factor" {
  description = "Replication factor for data"
  type        = number
  default     = 2
  
  validation {
    condition     = var.replication_factor >= 1
    error_message = "Replication factor must be at least 1."
  }
}

variable "storage_class" {
  description = "Storage class for persistent volumes"
  type        = string
  default     = "gp3"
}

variable "storage_size" {
  description = "Size of persistent volume for each node"
  type        = string
  default     = "20Gi"
}

variable "api_port" {
  description = "Port for HTTP API"
  type        = number
  default     = 8080
}

variable "grpc_port" {
  description = "Port for gRPC API"
  type        = number
  default     = 9090
}

variable "metrics_port" {
  description = "Port for metrics endpoint"
  type        = number
  default     = 8081
}

variable "health_port" {
  description = "Port for health check endpoint"
  type        = number
  default     = 8082
}

variable "log_level" {
  description = "Log level for the application"
  type        = string
  default     = "INFO"
  
  validation {
    condition     = contains(["DEBUG", "INFO", "WARN", "ERROR"], var.log_level)
    error_message = "Log level must be one of: DEBUG, INFO, WARN, ERROR."
  }
}

variable "monitoring_enabled" {
  description = "Enable monitoring and metrics collection"
  type        = bool
  default     = true
}

variable "tracing_enabled" {
  description = "Enable distributed tracing"
  type        = bool
  default     = true
}

variable "service_type" {
  description = "Kubernetes service type"
  type        = string
  default     = "LoadBalancer"
  
  validation {
    condition     = contains(["ClusterIP", "NodePort", "LoadBalancer"], var.service_type)
    error_message = "Service type must be ClusterIP, NodePort, or LoadBalancer."
  }
}

variable "resource_requests" {
  description = "Resource requests for KVStore containers"
  type        = map(string)
  default = {
    cpu    = "200m"
    memory = "512Mi"
  }
}

variable "resource_limits" {
  description = "Resource limits for KVStore containers"
  type        = map(string)
  default = {
    cpu    = "1000m"
    memory = "2Gi"
  }
}

variable "encryption_key" {
  description = "Encryption key for data at rest"
  type        = string
  sensitive   = true
}

variable "tls_cert" {
  description = "TLS certificate for secure communication"
  type        = string
  default     = ""
}

variable "tls_key" {
  description = "TLS private key for secure communication"
  type        = string
  default     = ""
  sensitive   = true
}

variable "service_account_annotations" {
  description = "Annotations for the service account"
  type        = map(string)
  default     = {}
}

variable "load_balancer_annotations" {
  description = "Annotations for the load balancer service"
  type        = map(string)
  default     = {}
}

variable "node_selector" {
  description = "Node selector for pod placement"
  type        = map(string)
  default     = {}
}

variable "tolerations" {
  description = "Tolerations for pod scheduling"
  type = list(object({
    key      = string
    operator = string
    value    = string
    effect   = string
  }))
  default = []
}