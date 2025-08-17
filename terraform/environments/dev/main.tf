# Development Environment Configuration

terraform {
  required_version = ">= 1.0"
  
  backend "s3" {
    bucket         = "kvstore-terraform-state-dev"
    key            = "dev/terraform.tfstate"
    region         = "us-west-2"
    encrypt        = true
    dynamodb_table = "kvstore-terraform-locks"
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.0"
    }
  }
}

# Configure providers
provider "aws" {
  region = var.aws_region
  
  default_tags {
    tags = {
      Environment = "dev"
      Project     = "distributed-kvstore"
      ManagedBy   = "terraform"
      Owner       = "platform-team"
    }
  }
}

data "aws_eks_cluster" "cluster" {
  name = module.eks.cluster_id
}

data "aws_eks_cluster_auth" "cluster" {
  name = module.eks.cluster_id
}

provider "kubernetes" {
  host                   = data.aws_eks_cluster.cluster.endpoint
  cluster_ca_certificate = base64decode(data.aws_eks_cluster.cluster.certificate_authority[0].data)
  token                  = data.aws_eks_cluster_auth.cluster.token
}

provider "helm" {
  kubernetes {
    host                   = data.aws_eks_cluster.cluster.endpoint
    cluster_ca_certificate = base64decode(data.aws_eks_cluster.cluster.certificate_authority[0].data)
    token                  = data.aws_eks_cluster_auth.cluster.token
  }
}

# Local variables
locals {
  cluster_name = "kvstore-dev"
  environment  = "dev"
  
  tags = {
    Environment = local.environment
    Project     = "distributed-kvstore"
    ManagedBy   = "terraform"
  }
}

# EKS Cluster Module
module "eks" {
  source = "../../modules/aws-eks"

  cluster_name       = local.cluster_name
  environment        = local.environment
  kubernetes_version = "1.28"

  # Network configuration
  vpc_cidr        = "10.0.0.0/16"
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]

  # Node group configuration
  node_instance_types     = ["t3.medium", "t3.large"]
  node_capacity_type      = "SPOT"
  node_group_min_size     = 1
  node_group_max_size     = 5
  node_group_desired_size = 2
  node_disk_size          = 30

  # Access configuration
  cluster_endpoint_public_access       = true
  cluster_endpoint_public_access_cidrs = ["0.0.0.0/0"]

  # Logging
  log_retention_days = 3

  tags = local.tags
}

# Generate encryption key for development
resource "random_password" "encryption_key" {
  length  = 32
  special = false
}

# KVStore Application Module
module "kvstore_app" {
  source = "../../modules/kvstore-app"

  depends_on = [module.eks]

  namespace        = "kvstore-dev"
  app_version      = var.app_version
  image_repository = "ghcr.io/distributed-kvstore/kvstore"
  environment      = local.environment

  # Application configuration
  cluster_size       = 3
  replication_factor = 2
  storage_class      = "gp3"
  storage_size       = "10Gi"

  # Resource configuration (smaller for dev)
  resource_requests = {
    cpu    = "100m"
    memory = "256Mi"
  }
  resource_limits = {
    cpu    = "500m"
    memory = "1Gi"
  }

  # Security configuration
  encryption_key = random_password.encryption_key.result
  tls_cert       = ""
  tls_key        = ""

  # Service configuration
  service_type = "LoadBalancer"
  load_balancer_annotations = {
    "service.beta.kubernetes.io/aws-load-balancer-type"   = "nlb"
    "service.beta.kubernetes.io/aws-load-balancer-scheme" = "internet-facing"
  }

  # Monitoring
  monitoring_enabled = true
  tracing_enabled    = true
  log_level          = "DEBUG"
}

# Install essential cluster addons
resource "helm_release" "aws_load_balancer_controller" {
  name       = "aws-load-balancer-controller"
  repository = "https://aws.github.io/eks-charts"
  chart      = "aws-load-balancer-controller"
  namespace  = "kube-system"
  version    = "1.6.0"

  values = [
    templatefile("${path.module}/values/aws-load-balancer-controller.yaml", {
      cluster_name = module.eks.cluster_id
      region       = var.aws_region
      vpc_id       = module.eks.vpc_id
      role_arn     = module.eks.load_balancer_controller_irsa_arn
    })
  ]

  depends_on = [module.eks]
}

# Monitoring stack (lightweight for dev)
resource "helm_release" "prometheus_stack" {
  name       = "prometheus"
  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  namespace  = "monitoring"
  version    = "51.0.0"

  create_namespace = true

  values = [
    file("${path.module}/values/prometheus-stack-dev.yaml")
  ]

  depends_on = [module.eks]
}

# Backup configuration for development
resource "aws_s3_bucket" "backup" {
  bucket = "${local.cluster_name}-backups"

  tags = local.tags
}

resource "aws_s3_bucket_versioning" "backup" {
  bucket = aws_s3_bucket.backup.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "backup" {
  bucket = aws_s3_bucket.backup.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# Scheduled backup job
resource "kubernetes_cron_job" "backup" {
  metadata {
    name      = "kvstore-backup"
    namespace = module.kvstore_app.namespace
  }

  spec {
    schedule = "0 2 * * *"  # Daily at 2 AM
    
    job_template {
      metadata {
        name = "kvstore-backup"
      }
      
      spec {
        template {
          metadata {
            name = "kvstore-backup"
          }
          
          spec {
            restart_policy = "OnFailure"
            
            container {
              name  = "backup"
              image = "ghcr.io/distributed-kvstore/backup-tool:latest"
              
              env {
                name  = "BACKUP_S3_BUCKET"
                value = aws_s3_bucket.backup.bucket
              }
              
              env {
                name  = "ENVIRONMENT"
                value = local.environment
              }
              
              env {
                name  = "KVSTORE_ENDPOINT"
                value = "kvstore-lb:8080"
              }
            }
          }
        }
      }
    }
  }

  depends_on = [module.kvstore_app]
}