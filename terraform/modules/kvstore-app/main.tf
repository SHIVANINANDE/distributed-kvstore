# KVStore Application Terraform Module

terraform {
  required_version = ">= 1.0"
  required_providers {
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

# Namespace for KVStore
resource "kubernetes_namespace" "kvstore" {
  metadata {
    name = var.namespace
    labels = {
      "app.kubernetes.io/name"    = "kvstore"
      "app.kubernetes.io/version" = var.app_version
      environment                 = var.environment
    }
    annotations = {
      "cluster-autoscaler.kubernetes.io/safe-to-evict" = "false"
    }
  }
}

# ConfigMap for KVStore configuration
resource "kubernetes_config_map" "kvstore_config" {
  metadata {
    name      = "kvstore-config"
    namespace = kubernetes_namespace.kvstore.metadata[0].name
    labels = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "config"
    }
  }

  data = {
    "config.yaml" = templatefile("${path.module}/templates/config.yaml.tpl", {
      cluster_size         = var.cluster_size
      replication_factor   = var.replication_factor
      storage_class        = var.storage_class
      storage_size         = var.storage_size
      log_level           = var.log_level
      monitoring_enabled  = var.monitoring_enabled
      tracing_enabled     = var.tracing_enabled
      metrics_port        = var.metrics_port
      health_port         = var.health_port
    })
  }
}

# Secret for KVStore sensitive configuration
resource "kubernetes_secret" "kvstore_secrets" {
  metadata {
    name      = "kvstore-secrets"
    namespace = kubernetes_namespace.kvstore.metadata[0].name
    labels = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "secrets"
    }
  }

  type = "Opaque"

  data = {
    encryption_key = base64encode(var.encryption_key)
    tls_cert       = base64encode(var.tls_cert)
    tls_key        = base64encode(var.tls_key)
  }
}

# Service Account for KVStore
resource "kubernetes_service_account" "kvstore" {
  metadata {
    name      = "kvstore"
    namespace = kubernetes_namespace.kvstore.metadata[0].name
    labels = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "serviceaccount"
    }
    annotations = var.service_account_annotations
  }
}

# RBAC for KVStore
resource "kubernetes_cluster_role" "kvstore" {
  metadata {
    name = "kvstore-cluster-role"
    labels = {
      "app.kubernetes.io/name" = "kvstore"
    }
  }

  rule {
    api_groups = [""]
    resources  = ["nodes", "pods", "services", "endpoints"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["apps"]
    resources  = ["deployments", "statefulsets"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["kvstore.io"]
    resources  = ["kvstores", "kvstores/status"]
    verbs      = ["get", "list", "watch", "create", "update", "patch", "delete"]
  }
}

resource "kubernetes_cluster_role_binding" "kvstore" {
  metadata {
    name = "kvstore-cluster-role-binding"
    labels = {
      "app.kubernetes.io/name" = "kvstore"
    }
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.kvstore.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.kvstore.metadata[0].name
    namespace = kubernetes_namespace.kvstore.metadata[0].name
  }
}

# Persistent Volume Claims for KVStore
resource "kubernetes_persistent_volume_claim" "kvstore_data" {
  count = var.cluster_size

  metadata {
    name      = "kvstore-data-${count.index}"
    namespace = kubernetes_namespace.kvstore.metadata[0].name
    labels = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "storage"
      "kvstore.io/node-id"         = tostring(count.index)
    }
  }

  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = var.storage_class
    
    resources {
      requests = {
        storage = var.storage_size
      }
    }
  }

  wait_until_bound = false
}

# StatefulSet for KVStore
resource "kubernetes_stateful_set" "kvstore" {
  metadata {
    name      = "kvstore"
    namespace = kubernetes_namespace.kvstore.metadata[0].name
    labels = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "server"
      "app.kubernetes.io/version"   = var.app_version
    }
  }

  spec {
    replicas     = var.cluster_size
    service_name = kubernetes_service.kvstore_headless.metadata[0].name

    selector {
      match_labels = {
        "app.kubernetes.io/name"      = "kvstore"
        "app.kubernetes.io/component" = "server"
      }
    }

    template {
      metadata {
        labels = {
          "app.kubernetes.io/name"      = "kvstore"
          "app.kubernetes.io/component" = "server"
          "app.kubernetes.io/version"   = var.app_version
        }
        annotations = {
          "prometheus.io/scrape" = "true"
          "prometheus.io/port"   = tostring(var.metrics_port)
          "prometheus.io/path"   = "/metrics"
        }
      }

      spec {
        service_account_name             = kubernetes_service_account.kvstore.metadata[0].name
        termination_grace_period_seconds = 30
        
        security_context {
          fs_group = 1000
        }

        init_container {
          name  = "init-kvstore"
          image = "${var.image_repository}:${var.app_version}"
          
          command = ["/bin/sh", "-c"]
          args = [
            "echo 'Initializing KVStore node...' && sleep 5"
          ]

          volume_mount {
            name       = "kvstore-data"
            mount_path = "/data"
          }
        }

        container {
          name  = "kvstore"
          image = "${var.image_repository}:${var.app_version}"
          
          port {
            name           = "api"
            container_port = var.api_port
            protocol       = "TCP"
          }

          port {
            name           = "grpc"
            container_port = var.grpc_port
            protocol       = "TCP"
          }

          port {
            name           = "metrics"
            container_port = var.metrics_port
            protocol       = "TCP"
          }

          port {
            name           = "health"
            container_port = var.health_port
            protocol       = "TCP"
          }

          env {
            name = "NODE_ID"
            value_from {
              field_ref {
                field_path = "metadata.name"
              }
            }
          }

          env {
            name = "POD_IP"
            value_from {
              field_ref {
                field_path = "status.podIP"
              }
            }
          }

          env {
            name = "POD_NAMESPACE"
            value_from {
              field_ref {
                field_path = "metadata.namespace"
              }
            }
          }

          env {
            name  = "CLUSTER_SIZE"
            value = tostring(var.cluster_size)
          }

          env {
            name  = "ENVIRONMENT"
            value = var.environment
          }

          volume_mount {
            name       = "kvstore-data"
            mount_path = "/data"
          }

          volume_mount {
            name       = "kvstore-config"
            mount_path = "/etc/kvstore"
            read_only  = true
          }

          volume_mount {
            name       = "kvstore-secrets"
            mount_path = "/etc/kvstore/secrets"
            read_only  = true
          }

          liveness_probe {
            http_get {
              path = "/health"
              port = var.health_port
            }
            initial_delay_seconds = 30
            period_seconds        = 10
            timeout_seconds       = 5
            failure_threshold     = 3
          }

          readiness_probe {
            http_get {
              path = "/ready"
              port = var.health_port
            }
            initial_delay_seconds = 5
            period_seconds        = 5
            timeout_seconds       = 3
            failure_threshold     = 3
          }

          resources {
            requests = var.resource_requests
            limits   = var.resource_limits
          }

          security_context {
            run_as_non_root = true
            run_as_user     = 1000
            run_as_group    = 1000
            
            capabilities {
              drop = ["ALL"]
            }
            
            read_only_root_filesystem = true
          }
        }

        volume {
          name = "kvstore-config"
          config_map {
            name = kubernetes_config_map.kvstore_config.metadata[0].name
          }
        }

        volume {
          name = "kvstore-secrets"
          secret {
            secret_name = kubernetes_secret.kvstore_secrets.metadata[0].name
          }
        }

        dynamic "volume" {
          for_each = range(var.cluster_size)
          content {
            name = "kvstore-data"
            persistent_volume_claim {
              claim_name = kubernetes_persistent_volume_claim.kvstore_data[volume.value].metadata[0].name
            }
          }
        }

        affinity {
          pod_anti_affinity {
            preferred_during_scheduling_ignored_during_execution {
              weight = 100
              pod_affinity_term {
                label_selector {
                  match_expressions {
                    key      = "app.kubernetes.io/name"
                    operator = "In"
                    values   = ["kvstore"]
                  }
                }
                topology_key = "kubernetes.io/hostname"
              }
            }
          }
        }

        toleration {
          key      = "kvstore.io/dedicated"
          operator = "Equal"
          value    = "true"
          effect   = "NoSchedule"
        }
      }
    }

    update_strategy {
      type = "RollingUpdate"
      rolling_update {
        partition = 0
      }
    }
  }
}

# Headless Service for StatefulSet
resource "kubernetes_service" "kvstore_headless" {
  metadata {
    name      = "kvstore-headless"
    namespace = kubernetes_namespace.kvstore.metadata[0].name
    labels = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "service"
    }
  }

  spec {
    cluster_ip = "None"
    
    selector = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "server"
    }

    port {
      name        = "api"
      port        = var.api_port
      target_port = var.api_port
      protocol    = "TCP"
    }

    port {
      name        = "grpc"
      port        = var.grpc_port
      target_port = var.grpc_port
      protocol    = "TCP"
    }
  }
}

# Load Balancer Service
resource "kubernetes_service" "kvstore_lb" {
  metadata {
    name      = "kvstore-lb"
    namespace = kubernetes_namespace.kvstore.metadata[0].name
    labels = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "loadbalancer"
    }
    annotations = var.load_balancer_annotations
  }

  spec {
    type = var.service_type
    
    selector = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "server"
    }

    port {
      name        = "api"
      port        = var.api_port
      target_port = var.api_port
      protocol    = "TCP"
    }

    port {
      name        = "grpc"
      port        = var.grpc_port
      target_port = var.grpc_port
      protocol    = "TCP"
    }
  }
}

# Service Monitor for Prometheus
resource "kubernetes_manifest" "kvstore_service_monitor" {
  count = var.monitoring_enabled ? 1 : 0

  manifest = {
    apiVersion = "monitoring.coreos.com/v1"
    kind       = "ServiceMonitor"
    metadata = {
      name      = "kvstore"
      namespace = kubernetes_namespace.kvstore.metadata[0].name
      labels = {
        "app.kubernetes.io/name"      = "kvstore"
        "app.kubernetes.io/component" = "monitoring"
      }
    }
    spec = {
      selector = {
        matchLabels = {
          "app.kubernetes.io/name"      = "kvstore"
          "app.kubernetes.io/component" = "server"
        }
      }
      endpoints = [
        {
          port     = "metrics"
          interval = "30s"
          path     = "/metrics"
        }
      ]
    }
  }
}