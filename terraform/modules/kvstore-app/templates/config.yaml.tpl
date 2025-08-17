# KVStore Configuration Template

cluster:
  size: ${cluster_size}
  replicationFactor: ${replication_factor}
  
storage:
  engine: "badger"
  dataPath: "/data"
  class: "${storage_class}"
  size: "${storage_size}"
  syncWrites: true
  compressionEnabled: true
  encryptionEnabled: true

network:
  api:
    port: 8080
    bindAddress: "0.0.0.0"
    tlsEnabled: true
  grpc:
    port: 9090
    bindAddress: "0.0.0.0"
    tlsEnabled: true
  discovery:
    method: "kubernetes"
    namespace: "kvstore"
    serviceName: "kvstore-headless"

logging:
  level: "${log_level}"
  format: "json"
  output: "stdout"
  structured: true

monitoring:
  enabled: ${monitoring_enabled}
  metricsPort: ${metrics_port}
  healthPort: ${health_port}
  prometheusEnabled: true
  
tracing:
  enabled: ${tracing_enabled}
  jaegerEndpoint: "http://jaeger-collector:14268/api/traces"
  samplingRate: 0.1

raft:
  electionTimeout: "1s"
  heartbeatTimeout: "500ms"
  leaderLeaseTimeout: "500ms"
  commitTimeout: "50ms"
  maxAppendEntries: 64
  snapshotInterval: "120s"
  snapshotThreshold: 8192

security:
  authenticationEnabled: true
  authorizationEnabled: true
  tlsCertFile: "/etc/kvstore/secrets/tls_cert"
  tlsKeyFile: "/etc/kvstore/secrets/tls_key"
  encryptionKeyFile: "/etc/kvstore/secrets/encryption_key"

performance:
  maxConnections: 1000
  readTimeout: "30s"
  writeTimeout: "30s"
  idleTimeout: "60s"
  batchSize: 100
  
chaos:
  enabled: false
  scenarios:
    - name: "network_partition"
      probability: 0.01
    - name: "node_failure"
      probability: 0.005