# Variables for Disaster Recovery Module

variable "cluster_name" {
  description = "Name of the EKS cluster"
  type        = string
}

variable "environment" {
  description = "Environment name (dev, staging, prod)"
  type        = string
}

variable "namespace" {
  description = "Kubernetes namespace"
  type        = string
  default     = "kvstore"
}

variable "backup_schedule" {
  description = "Cron schedule for backups"
  type        = string
  default     = "0 2 * * *"
}

variable "primary_region" {
  description = "Primary AWS region"
  type        = string
}

variable "backup_regions" {
  description = "List of backup regions for cross-region replication"
  type        = list(string)
  default     = []
}

variable "backup_retention_days" {
  description = "Number of days to retain backups"
  type        = number
  default     = 30
}

variable "snapshot_retention_days" {
  description = "Number of days to retain EBS snapshots"
  type        = number
  default     = 7
}

variable "oidc_provider_arn" {
  description = "ARN of the OIDC provider for EKS"
  type        = string
}

variable "oidc_issuer_url" {
  description = "URL of the OIDC issuer for EKS"
  type        = string
}

variable "sns_topic_arn" {
  description = "SNS topic ARN for notifications"
  type        = string
  default     = ""
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = map(string)
  default     = {}
}