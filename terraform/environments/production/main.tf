# Production Environment Configuration

terraform {
  required_version = ">= 1.0"
  
  backend "s3" {
    bucket         = "kvstore-terraform-state-prod"
    key            = "prod/terraform.tfstate"
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
      Environment = "production"
      Project     = "distributed-kvstore"
      ManagedBy   = "terraform"
      Owner       = "platform-team"
      Criticality = "high"
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
  cluster_name = "kvstore-prod"
  environment  = "production"
  
  tags = {
    Environment = local.environment
    Project     = "distributed-kvstore"
    ManagedBy   = "terraform"
    Criticality = "high"
  }
}

# Multi-AZ EKS Cluster Module
module "eks" {
  source = "../../modules/aws-eks"

  cluster_name       = local.cluster_name
  environment        = local.environment
  kubernetes_version = "1.28"

  # Network configuration - Multi-AZ for HA
  vpc_cidr        = "10.1.0.0/16"
  private_subnets = ["10.1.1.0/24", "10.1.2.0/24", "10.1.3.0/24"]
  public_subnets  = ["10.1.101.0/24", "10.1.102.0/24", "10.1.103.0/24"]

  # Node group configuration - Production sized
  node_instance_types     = ["m5.xlarge", "m5.2xlarge"]
  node_capacity_type      = "ON_DEMAND"
  node_group_min_size     = 3
  node_group_max_size     = 20
  node_group_desired_size = 6
  node_disk_size          = 100

  # Security configuration
  cluster_endpoint_public_access       = false
  cluster_endpoint_public_access_cidrs = []

  # Logging - Extended retention for production
  log_retention_days = 30

  # Additional AWS auth mappings
  aws_auth_roles = [
    {
      rolearn  = var.admin_role_arn
      username = "admin"
      groups   = ["system:masters"]
    },
    {
      rolearn  = var.developer_role_arn
      username = "developer"
      groups   = ["system:basic-user"]
    }
  ]

  tags = local.tags
}

# Production KVStore Application with High Availability
module "kvstore_app" {
  source = "../../modules/kvstore-app"

  depends_on = [module.eks]

  namespace        = "kvstore"
  app_version      = var.app_version
  image_repository = "ghcr.io/distributed-kvstore/kvstore"
  environment      = local.environment

  # High availability configuration
  cluster_size       = 5
  replication_factor = 3
  storage_class      = "gp3"
  storage_size       = "100Gi"

  # Production resource allocation
  resource_requests = {
    cpu    = "500m"
    memory = "2Gi"
  }
  resource_limits = {
    cpu    = "2000m"
    memory = "8Gi"
  }

  # Security configuration
  encryption_key = var.encryption_key
  tls_cert       = var.tls_cert
  tls_key        = var.tls_key

  # Service configuration with internal load balancer
  service_type = "LoadBalancer"
  load_balancer_annotations = {
    "service.beta.kubernetes.io/aws-load-balancer-type"     = "nlb"
    "service.beta.kubernetes.io/aws-load-balancer-scheme"   = "internal"
    "service.beta.kubernetes.io/aws-load-balancer-backend-protocol" = "tcp"
  }

  # Monitoring and observability
  monitoring_enabled = true
  tracing_enabled    = true
  log_level          = "INFO"
}

# Production monitoring stack
resource "helm_release" "prometheus_stack" {
  name       = "prometheus"
  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  namespace  = "monitoring"
  version    = "51.0.0"

  create_namespace = true

  values = [
    templatefile("${path.module}/values/prometheus-stack-prod.yaml", {
      retention_days = "30d"
      storage_size   = "100Gi"
      replicas       = 2
    })
  ]

  depends_on = [module.eks]
}

# Disaster Recovery Infrastructure
module "disaster_recovery" {
  source = "../../modules/disaster-recovery"

  cluster_name    = local.cluster_name
  environment     = local.environment
  backup_schedule = "0 */6 * * *"  # Every 6 hours
  
  # Multi-region backup
  primary_region   = var.aws_region
  backup_regions   = ["us-east-1", "eu-west-1"]
  
  # Retention policies
  backup_retention_days = 90
  snapshot_retention_days = 30
  
  tags = local.tags
}

# Network policies for production security
resource "kubernetes_network_policy" "kvstore_isolation" {
  metadata {
    name      = "kvstore-network-policy"
    namespace = module.kvstore_app.namespace
  }

  spec {
    pod_selector {
      match_labels = {
        "app.kubernetes.io/name" = "kvstore"
      }
    }

    policy_types = ["Ingress", "Egress"]

    ingress {
      from {
        namespace_selector {
          match_labels = {
            name = "ingress-nginx"
          }
        }
      }
      
      from {
        namespace_selector {
          match_labels = {
            name = "monitoring"
          }
        }
      }

      ports {
        protocol = "TCP"
        port     = "8080"
      }
      
      ports {
        protocol = "TCP"
        port     = "9090"
      }
    }

    egress {
      # Allow DNS resolution
      to {}
      ports {
        protocol = "UDP"
        port     = "53"
      }
    }

    egress {
      # Allow internal cluster communication
      to {
        namespace_selector {
          match_labels = {
            name = module.kvstore_app.namespace
          }
        }
      }
    }
  }
}

# Pod disruption budget for high availability
resource "kubernetes_pod_disruption_budget" "kvstore" {
  metadata {
    name      = "kvstore-pdb"
    namespace = module.kvstore_app.namespace
  }

  spec {
    min_available = 3
    
    selector {
      match_labels = {
        "app.kubernetes.io/name"      = "kvstore"
        "app.kubernetes.io/component" = "server"
      }
    }
  }
}

# Horizontal Pod Autoscaler
resource "kubernetes_horizontal_pod_autoscaler_v2" "kvstore" {
  metadata {
    name      = "kvstore-hpa"
    namespace = module.kvstore_app.namespace
  }

  spec {
    scale_target_ref {
      api_version = "apps/v1"
      kind        = "StatefulSet"
      name        = "kvstore"
    }

    min_replicas = 5
    max_replicas = 10

    metric {
      type = "Resource"
      resource {
        name = "cpu"
        target {
          type                = "Utilization"
          average_utilization = 70
        }
      }
    }

    metric {
      type = "Resource"
      resource {
        name = "memory"
        target {
          type                = "Utilization"
          average_utilization = 80
        }
      }
    }
  }
}

# Alerting rules
resource "kubernetes_config_map" "alerting_rules" {
  metadata {
    name      = "kvstore-alerts"
    namespace = "monitoring"
    labels = {
      "app.kubernetes.io/name" = "prometheus"
    }
  }

  data = {
    "kvstore.yaml" = file("${path.module}/alerts/kvstore-alerts.yaml")
  }
}