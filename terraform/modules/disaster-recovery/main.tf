# Disaster Recovery Module

terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
  }
}

# Primary backup S3 bucket
resource "aws_s3_bucket" "primary_backup" {
  bucket = "${var.cluster_name}-backups-${var.primary_region}"

  tags = merge(var.tags, {
    Purpose = "primary-backup"
    Region  = var.primary_region
  })
}

resource "aws_s3_bucket_versioning" "primary_backup" {
  bucket = aws_s3_bucket.primary_backup.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "primary_backup" {
  bucket = aws_s3_bucket.primary_backup.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.backup.arn
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "primary_backup" {
  bucket = aws_s3_bucket.primary_backup.id

  rule {
    id     = "backup_lifecycle"
    status = "Enabled"

    expiration {
      days = var.backup_retention_days
    }

    noncurrent_version_expiration {
      noncurrent_days = 30
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

# Cross-region replication buckets
resource "aws_s3_bucket" "replica_backup" {
  count = length(var.backup_regions)

  provider = aws.replica
  bucket   = "${var.cluster_name}-backups-${var.backup_regions[count.index]}"

  tags = merge(var.tags, {
    Purpose = "replica-backup"
    Region  = var.backup_regions[count.index]
  })
}

resource "aws_s3_bucket_replication_configuration" "backup_replication" {
  role   = aws_iam_role.replication.arn
  bucket = aws_s3_bucket.primary_backup.id

  dynamic "rule" {
    for_each = var.backup_regions
    content {
      id     = "replicate-to-${rule.value}"
      status = "Enabled"

      destination {
        bucket        = aws_s3_bucket.replica_backup[rule.key].arn
        storage_class = "STANDARD_IA"

        encryption_configuration {
          replica_kms_key_id = aws_kms_key.backup.arn
        }
      }
    }
  }

  depends_on = [aws_s3_bucket_versioning.primary_backup]
}

# KMS key for backup encryption
resource "aws_kms_key" "backup" {
  description             = "KMS key for ${var.cluster_name} backup encryption"
  deletion_window_in_days = 7
  enable_key_rotation     = true

  tags = var.tags
}

resource "aws_kms_alias" "backup" {
  name          = "alias/${var.cluster_name}-backup"
  target_key_id = aws_kms_key.backup.key_id
}

# IAM role for S3 replication
resource "aws_iam_role" "replication" {
  name = "${var.cluster_name}-s3-replication"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "s3.amazonaws.com"
        }
      }
    ]
  })

  tags = var.tags
}

resource "aws_iam_role_policy" "replication" {
  name = "${var.cluster_name}-s3-replication"
  role = aws_iam_role.replication.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "s3:GetObjectVersionForReplication",
          "s3:GetObjectVersionAcl",
          "s3:GetObjectVersionTagging"
        ]
        Effect = "Allow"
        Resource = [
          "${aws_s3_bucket.primary_backup.arn}/*"
        ]
      },
      {
        Action = [
          "s3:ListBucket"
        ]
        Effect = "Allow"
        Resource = [
          aws_s3_bucket.primary_backup.arn
        ]
      },
      {
        Action = [
          "s3:ReplicateObject",
          "s3:ReplicateDelete",
          "s3:ReplicateTags"
        ]
        Effect = "Allow"
        Resource = [
          for bucket in aws_s3_bucket.replica_backup : "${bucket.arn}/*"
        ]
      },
      {
        Action = [
          "kms:Decrypt",
          "kms:DescribeKey"
        ]
        Effect = "Allow"
        Resource = [
          aws_kms_key.backup.arn
        ]
      }
    ]
  })
}

# EBS snapshot automation
resource "aws_dlm_lifecycle_policy" "kvstore_snapshots" {
  description        = "KVStore EBS snapshot policy"
  execution_role_arn = aws_iam_role.dlm.arn
  state              = "ENABLED"

  policy_details {
    resource_types   = ["VOLUME"]
    target_tags = {
      "kubernetes.io/cluster/${var.cluster_name}" = "owned"
      "CSIVolumeName"                              = "*"
    }

    schedule {
      name = "Daily snapshots"

      create_rule {
        interval      = 24
        interval_unit = "HOURS"
        times         = ["03:00"]
      }

      retain_rule {
        count = var.snapshot_retention_days
      }

      tags_to_add = merge(var.tags, {
        SnapshotCreator = "DLM"
        Application     = "kvstore"
      })

      copy_tags = true
    }
  }

  tags = var.tags
}

# IAM role for DLM
resource "aws_iam_role" "dlm" {
  name = "${var.cluster_name}-dlm"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "dlm.amazonaws.com"
        }
      }
    ]
  })

  tags = var.tags
}

resource "aws_iam_role_policy_attachment" "dlm" {
  role       = aws_iam_role.dlm.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSDataLifecycleManagerServiceRole"
}

# Backup job using Kubernetes CronJob
resource "kubernetes_service_account" "backup" {
  metadata {
    name      = "kvstore-backup"
    namespace = var.namespace
    annotations = {
      "eks.amazonaws.com/role-arn" = aws_iam_role.backup_service.arn
    }
  }
}

resource "aws_iam_role" "backup_service" {
  name = "${var.cluster_name}-backup-service"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRoleWithWebIdentity"
        Effect = "Allow"
        Principal = {
          Federated = var.oidc_provider_arn
        }
        Condition = {
          StringEquals = {
            "${replace(var.oidc_issuer_url, "https://", "")}:sub": "system:serviceaccount:${var.namespace}:kvstore-backup"
            "${replace(var.oidc_issuer_url, "https://", "")}:aud": "sts.amazonaws.com"
          }
        }
      }
    ]
  })

  tags = var.tags
}

resource "aws_iam_role_policy" "backup_service" {
  name = "${var.cluster_name}-backup-service"
  role = aws_iam_role.backup_service.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:PutObject",
          "s3:PutObjectAcl",
          "s3:GetObject",
          "s3:GetObjectAcl",
          "s3:DeleteObject",
          "s3:ListBucket"
        ]
        Resource = [
          aws_s3_bucket.primary_backup.arn,
          "${aws_s3_bucket.primary_backup.arn}/*"
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "kms:Encrypt",
          "kms:Decrypt",
          "kms:ReEncrypt*",
          "kms:GenerateDataKey*",
          "kms:DescribeKey"
        ]
        Resource = [
          aws_kms_key.backup.arn
        ]
      }
    ]
  })
}

resource "kubernetes_cron_job" "backup" {
  metadata {
    name      = "kvstore-backup"
    namespace = var.namespace
    labels = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "backup"
    }
  }

  spec {
    schedule = var.backup_schedule
    
    job_template {
      metadata {
        name = "kvstore-backup"
      }
      
      spec {
        backoff_limit = 3
        
        template {
          metadata {
            name = "kvstore-backup"
          }
          
          spec {
            service_account_name = kubernetes_service_account.backup.metadata[0].name
            restart_policy       = "OnFailure"
            
            container {
              name  = "backup"
              image = "ghcr.io/distributed-kvstore/backup:latest"
              
              env {
                name  = "BACKUP_S3_BUCKET"
                value = aws_s3_bucket.primary_backup.bucket
              }
              
              env {
                name  = "BACKUP_S3_PREFIX"
                value = "kvstore/${var.environment}"
              }
              
              env {
                name  = "ENVIRONMENT"
                value = var.environment
              }
              
              env {
                name  = "KVSTORE_ENDPOINTS"
                value = "kvstore-0.kvstore-headless:8080,kvstore-1.kvstore-headless:8080,kvstore-2.kvstore-headless:8080"
              }
              
              env {
                name  = "BACKUP_TYPE"
                value = "full"
              }
              
              env {
                name  = "RETENTION_DAYS"
                value = tostring(var.backup_retention_days)
              }
              
              resources {
                requests = {
                  cpu    = "100m"
                  memory = "256Mi"
                }
                limits = {
                  cpu    = "500m"
                  memory = "1Gi"
                }
              }
              
              security_context {
                run_as_non_root = true
                run_as_user     = 1000
                
                capabilities {
                  drop = ["ALL"]
                }
                
                read_only_root_filesystem = true
              }
            }
          }
        }
      }
    }
  }
}

# Recovery testing job (runs weekly)
resource "kubernetes_cron_job" "recovery_test" {
  metadata {
    name      = "kvstore-recovery-test"
    namespace = var.namespace
    labels = {
      "app.kubernetes.io/name"      = "kvstore"
      "app.kubernetes.io/component" = "recovery-test"
    }
  }

  spec {
    schedule = "0 4 * * 0"  # Weekly on Sunday at 4 AM
    
    job_template {
      metadata {
        name = "kvstore-recovery-test"
      }
      
      spec {
        backoff_limit = 1
        
        template {
          metadata {
            name = "kvstore-recovery-test"
          }
          
          spec {
            service_account_name = kubernetes_service_account.backup.metadata[0].name
            restart_policy       = "OnFailure"
            
            container {
              name  = "recovery-test"
              image = "ghcr.io/distributed-kvstore/recovery-test:latest"
              
              env {
                name  = "BACKUP_S3_BUCKET"
                value = aws_s3_bucket.primary_backup.bucket
              }
              
              env {
                name  = "TEST_NAMESPACE"
                value = "${var.namespace}-recovery-test"
              }
              
              env {
                name  = "ENVIRONMENT"
                value = var.environment
              }
              
              resources {
                requests = {
                  cpu    = "200m"
                  memory = "512Mi"
                }
                limits = {
                  cpu    = "1000m"
                  memory = "2Gi"
                }
              }
            }
          }
        }
      }
    }
  }
}

# CloudWatch alarms for disaster recovery monitoring
resource "aws_cloudwatch_metric_alarm" "backup_failure" {
  alarm_name          = "${var.cluster_name}-backup-failure"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "1"
  metric_name         = "Failed"
  namespace           = "AWS/DLM"
  period              = "300"
  statistic           = "Sum"
  threshold           = "0"
  alarm_description   = "This metric monitors backup job failures"
  alarm_actions       = [var.sns_topic_arn]

  dimensions = {
    PolicyId = aws_dlm_lifecycle_policy.kvstore_snapshots.id
  }

  tags = var.tags
}

resource "aws_cloudwatch_metric_alarm" "replication_failure" {
  alarm_name          = "${var.cluster_name}-replication-failure"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "2"
  metric_name         = "ReplicationLatency"
  namespace           = "AWS/S3"
  period              = "900"
  statistic           = "Average"
  threshold           = "900"  # 15 minutes
  alarm_description   = "This metric monitors S3 replication latency"
  alarm_actions       = [var.sns_topic_arn]

  dimensions = {
    SourceBucket = aws_s3_bucket.primary_backup.bucket
  }

  tags = var.tags
}