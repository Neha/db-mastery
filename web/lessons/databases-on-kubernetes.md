## Should you run databases on Kubernetes?

Five years ago the answer was "no". Today, with mature **operators** and CSI storage, many enterprises run Postgres, MongoDB, Kafka and Redis on Kubernetes and **OpenShift**, especially for **hybrid cloud** (the same platform on-prem and in every cloud), self-service for developers, and consistency with GitOps.

| Run on Kubernetes when… | Use a managed DBaaS when… |
|---|---|
| Hybrid / on-prem / sovereign / air-gapped requirements | All-in on one public cloud |
| Platform team wants one operating model for everything | Small team, no Kubernetes expertise |
| Many small databases (dev/test, per-team, per-tenant) | Few, very large, critical databases |
| Cost control at scale, avoiding DBaaS premiums | Need vendor-managed SLAs and features |

> **Interview:** When would you tell a customer *not* to run the database on Kubernetes?

> **Model answer:** When they are all-in on one public cloud, have a small team with no Kubernetes expertise, and need a vendor SLA for a few large, critical databases — use managed DBaaS. Also decline when storage is NFS, there is no operator (StatefulSet-only), or they cannot test failover and PITR. Hybrid, sovereign, and many small self-service databases are the cases for operators on OpenShift.

> **SA angle:** This table *is* the Red Hat pitch for data workloads. OpenShift provides the consistent platform everywhere, and certified operators make databases self-service. Expect "should we run databases on OpenShift?". Answer with trade-offs, not dogma.

## Kubernetes building blocks for stateful workloads

| Concept | Role for databases |
|---|---|
| **Pod** | Runs the database container |
| **StatefulSet** | Stable network identity (`pg-0`, `pg-1`), ordered start-up, one PVC per replica |
| **Headless Service** | Stable DNS per pod (`pg-0.pg.ns.svc`) for replication topology |
| **PersistentVolumeClaim / PersistentVolume** | Durable storage that survives pod restarts |
| **StorageClass / CSI driver** | Dynamic provisioning (EBS gp3, Ceph RBD, vSphere, NetApp…), snapshots |
| **PodDisruptionBudget** | Don't evict too many replicas during node maintenance |
| **Anti-affinity / topology spread** | Put replicas on different nodes and zones |
| **Resource requests and limits** | Guaranteed QoS for predictable performance |
| **Secrets** | Credentials (ideally from Vault or External Secrets) |

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata: { name: pg }
spec:
  serviceName: pg
  replicas: 3
  selector: { matchLabels: { app: pg } }
  template:
    metadata: { labels: { app: pg } }
    spec:
      containers:
        - name: postgres
          image: registry.redhat.io/rhel9/postgresql-16
          ports: [{ containerPort: 5432 }]
          volumeMounts: [{ name: data, mountPath: /var/lib/pgsql/data }]
  volumeClaimTemplates:
    - metadata: { name: data }
      spec:
        accessModes: [ReadWriteOnce]
        storageClassName: gp3-csi
        resources: { requests: { storage: 100Gi } }
```

A StatefulSet alone gives you **pods with disks**, not a **highly available database**. It knows nothing about promoting replicas, backups or upgrades. That's what operators add.

![The operator watches a Cluster CR and reconciles a StatefulSet: each ordinal pod has its own PVC. Identity and disks come from the StatefulSet; failover and backups come from the operator.](diagrams/databases-on-kubernetes-statefulset.svg)

## Operators: encoding the DBA in software

An **Operator** is a controller plus **Custom Resource Definitions (CRDs)**. You declare *what* you want, and the operator's reconciliation loop continuously makes it so:

```text
desired state (CR) ──▶ operator watches ──▶ compares with actual ──▶ acts (create, failover, backup, upgrade)
                              ▲                                            │
                              └──────────────── loop forever ──────────────┘
```

This is the same pattern as React's reconciliation: declare the desired UI, and the framework diffs and patches. You already understand operators conceptually.

```yaml
# CloudNativePG: a 3-instance HA Postgres cluster with backups to object storage
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata: { name: orders-db }
spec:
  instances: 3
  storage: { size: 100Gi, storageClass: ocs-storagecluster-ceph-rbd }
  postgresql:
    parameters: { shared_buffers: "2GB", max_connections: "200" }
  backup:
    barmanObjectStore:
      destinationPath: s3://backups/orders-db
      s3Credentials: { inheritFromIAMRole: true }
    retentionPolicy: "30d"
  monitoring: { enablePodMonitor: true }
```

### Operator capability levels (Operator Framework)

1. Basic install
2. Seamless upgrades
3. Full lifecycle (backup, failure recovery)
4. Deep insights (metrics, alerts)
5. Auto-pilot (auto-scaling, auto-tuning)

### Common database operators

| Database | Operators |
|---|---|
| PostgreSQL | **CloudNativePG** (CNCF), Crunchy PGO, EDB Postgres for Kubernetes, Zalando |
| MongoDB | **MongoDB Controllers for Kubernetes** (Enterprise and Community; also manage Atlas resources), Percona |
| Kafka | **Strimzi**, the basis of Red Hat Streams for Apache Kafka |
| Redis | Redis Enterprise Operator |
| MySQL | Oracle MySQL Operator, Percona, Vitess |

On OpenShift, operators are installed from **OperatorHub** via the **Operator Lifecycle Manager (OLM)**, with Red Hat **certified** and supported options.

## Storage on OpenShift

- **OpenShift Data Foundation (ODF)**, built on Ceph via Rook: block (RBD) for databases, file (CephFS) and object (S3-compatible via NooBaa/RGW). Provides consistent storage on-prem and in the cloud, plus replication for DR.
- Cloud CSI drivers (EBS, Azure Disk, PD) when running on public cloud.
- Use **block storage with ReadWriteOnce** for databases. Avoid NFS for database data files.

## Backup, restore and DR

| Layer | Tool |
|---|---|
| Database-native | pgBackRest/Barman (Postgres), mongodump / Ops Manager (MongoDB). App-consistent, enables PITR |
| Kubernetes-level | **OADP** (OpenShift API for Data Protection, based on Velero): namespace resources plus PV snapshots |
| Storage-level | CSI snapshots; ODF **Regional-DR / Metro-DR** for cross-cluster replication |

Always define **RPO** (how much data can you lose?) and **RTO** (how long can you be down?), and test restores regularly. A backup you haven't restored isn't a backup.

## Day-2 operations checklist

- Monitoring: Prometheus/Grafana (built into OpenShift), database exporters, alerts on replication lag, disk and connections
- Rolling upgrades of database minor and major versions via the operator
- Security: SCCs (OpenShift Security Context Constraints, non-root containers), NetworkPolicies, TLS everywhere, secrets management, image provenance
- GitOps: Argo CD (**OpenShift GitOps**) manages database CRs declaratively
- Capacity: storage expansion via PVC resize, vertical scaling via the CR

> **Hands-on:** Create a free **Red Hat Developer Sandbox** (OpenShift). From OperatorHub, install CloudNativePG (or Crunchy) into your namespace if allowed, or deploy a simple Postgres template. Create a cluster CR, delete the primary pod, and watch failover happen. Write the RTO you observed in your notes.

```quiz
Q: What does a Kubernetes Operator add beyond a StatefulSet for databases?
- Persistent volumes
- Stable pod names
+ Database-specific automation: failover, backups, upgrades and configuration, via a reconciliation loop over custom resources
- Load balancing
? StatefulSets give identity and storage. Operators encode operational knowledge (the "DBA in software").

Q: Which Kubernetes object gives each database replica a stable identity and its own persistent volume?
- Deployment
+ StatefulSet
- DaemonSet
- ReplicaSet
? StatefulSets provide ordinal names, stable DNS via headless services, and per-pod PVCs through volumeClaimTemplates.

Q: On OpenShift, which tool provides Kubernetes-level backup and restore of namespaces and persistent volumes?
- Strimzi
+ OADP (OpenShift API for Data Protection, based on Velero)
- Patroni
- PgBouncer
? OADP backs up Kubernetes resources and volume snapshots. Pair it with database-native backups for consistency and PITR.

Q: What is the Red Hat-backed operator for Apache Kafka?
- CloudNativePG
+ Strimzi (the upstream for Red Hat Streams for Apache Kafka)
- Rook
- Velero
? Strimzi is a CNCF project. Red Hat's supported distribution is built on it.
```
