# Variables for AWS EKS Module

variable "cluster_name" {
  description = "Name of the EKS cluster"
  type        = string
}

variable "environment" {
  description = "Environment name (dev, staging, prod)"
  type        = string
}

variable "kubernetes_version" {
  description = "Kubernetes version"
  type        = string
  default     = "1.28"
}

variable "vpc_cidr" {
  description = "CIDR block for VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "private_subnets" {
  description = "Private subnet CIDR blocks"
  type        = list(string)
  default     = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
}

variable "public_subnets" {
  description = "Public subnet CIDR blocks"
  type        = list(string)
  default     = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
}

variable "cluster_endpoint_public_access" {
  description = "Enable public access to cluster endpoint"
  type        = bool
  default     = true
}

variable "cluster_endpoint_public_access_cidrs" {
  description = "List of CIDR blocks that can access the public cluster endpoint"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "node_instance_types" {
  description = "EC2 instance types for EKS nodes"
  type        = list(string)
  default     = ["m5.large", "m5.xlarge"]
}

variable "node_capacity_type" {
  description = "Capacity type for nodes (ON_DEMAND or SPOT)"
  type        = string
  default     = "ON_DEMAND"
}

variable "node_group_min_size" {
  description = "Minimum number of nodes in node group"
  type        = number
  default     = 1
}

variable "node_group_max_size" {
  description = "Maximum number of nodes in node group"
  type        = number
  default     = 10
}

variable "node_group_desired_size" {
  description = "Desired number of nodes in node group"
  type        = number
  default     = 3
}

variable "node_disk_size" {
  description = "Disk size for EKS nodes in GB"
  type        = number
  default     = 50
}

variable "key_pair_name" {
  description = "EC2 Key Pair name for SSH access"
  type        = string
  default     = null
}

variable "log_retention_days" {
  description = "CloudWatch log retention in days"
  type        = number
  default     = 7
}

variable "aws_auth_roles" {
  description = "List of role maps to add to the aws-auth configmap"
  type = list(object({
    rolearn  = string
    username = string
    groups   = list(string)
  }))
  default = []
}

variable "aws_auth_users" {
  description = "List of user maps to add to the aws-auth configmap"
  type = list(object({
    userarn  = string
    username = string
    groups   = list(string)
  }))
  default = []
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = map(string)
  default = {
    Terraform   = "true"
    Project     = "distributed-kvstore"
  }
}