# Variables for Development Environment

variable "aws_region" {
  description = "AWS region for deployment"
  type        = string
  default     = "us-west-2"
}

variable "app_version" {
  description = "Version of the KVStore application to deploy"
  type        = string
  default     = "latest"
}