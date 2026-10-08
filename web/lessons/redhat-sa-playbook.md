## What Red Hat SAs do

Red Hat Solutions Architects work with account teams to help enterprises adopt the **open hybrid cloud** portfolio: OpenShift (including OpenShift Virtualization and ROSA/ARO), RHEL, Ansible Automation Platform, Red Hat AI, and application and integration services. Typical activities include discovery, architecture workshops, demos on OpenShift, POCs, "art of the possible" sessions, partner co-selling, and helping customers build a modernisation roadmap.

For data-focused conversations you'll cover databases on OpenShift (via partners and operators), Kafka and Debezium, storage (ODF), AI platforms and automation.

## What they look for

1. **Kubernetes and OpenShift depth**: operators, networking, storage, security (SCCs, RBAC), GitOps, multi-cluster.
2. **Linux fundamentals**: RHEL, systemd, SELinux, performance, containers (Podman).
3. **Open source fluency**: upstream/downstream, community, licences, contribution model.
4. **Hybrid architecture**: consistent platforms across on-prem, cloud and edge; migration strategies.
5. **Automation mindset**: Ansible, infrastructure as code, GitOps.
6. **Consultative selling**: understanding business drivers (cost, risk, sovereignty, speed).
7. Increasingly, **AI platforms**: OpenShift AI, model serving with vLLM, MLOps.

## The core pitch (2 minutes)

> "Most enterprises now run across several clouds and their own data centres, with VMs, containers and, increasingly, AI workloads. Each environment has different tools, skills and security controls, which slows teams down and increases risk. Red Hat gives you **one consistent, open platform everywhere**: RHEL as the foundation, OpenShift to build, deploy and run applications, VMs and AI models in any environment, and Ansible to automate operations end to end. Because it's built on open source, you avoid lock-in, benefit from the largest innovation communities, and get enterprise support, security and lifecycle from Red Hat."

## Data-workload story (your niche)

As an SA with database depth, you can stand out by owning conversations like:

- "Should we run databases on OpenShift?" Give the trade-offs (from the Kubernetes lesson) and certified operators (Crunchy, EDB, CloudNativePG, MongoDB Enterprise, etc.).
- "How do we break the monolith's data?" Debezium CDC plus Kafka (Streams for Apache Kafka) plus the strangler-fig pattern.
- "How do we do AI on private data?" OpenShift AI plus vLLM plus pgvector or Milvus on OpenShift, with ODF storage.
- "How do we automate database operations at scale?" Ansible content collections for Postgres and Kubernetes plus GitOps with Argo CD.

## Discovery questions

- What does your infrastructure footprint look like (data centres, clouds, edge)?
- What's driving change: VMware costs, cloud costs, regulatory requirements, developer productivity, AI?
- How do you deploy applications today? How long from commit to production?
- How are databases provisioned and operated? Who owns them? How long does a new database take?
- What are your sovereignty or residency constraints?
- What's your skills profile (Linux, Kubernetes, Ansible)?
- Which applications are candidates for modernisation first?

## Objection handling

| Objection | Response approach |
|---|---|
| "Why pay for OpenShift when Kubernetes is free?" | Vanilla Kubernetes is a kernel, not a platform. Cover the TCO of assembling and securing registry, CI/CD, monitoring, ingress, service mesh, security and lifecycle upgrades, plus support and certified operators. OpenShift is an integrated, supported platform (OKD is its community upstream). |
| "We'll just use EKS / AKS / GKE." | Fine for a single cloud, but hybrid and multi-cloud consistency, on-prem and edge, VM workloads, a consistent security posture and one skill set matter. ROSA is OpenShift as a managed service on AWS. |
| "Databases shouldn't run on Kubernetes." | Valid for some workloads. Operators have matured, and there's huge value for many small DBs, dev/test, self-service and hybrid. Recommend case by case. Managed DBaaS remains a valid choice for the crown jewels. |
| "Open source is risky." | Red Hat's role is exactly de-risking it: support, security response, certifications, long lifecycles, indemnification. |
| "VMware works fine." | Explore cost and licensing changes, then OpenShift Virtualization as a low-disruption path: migrate VMs (Migration Toolkit for Virtualization), then modernise gradually. |

## Demo / POC ideas

1. **Database self-service on OpenShift**: install CloudNativePG or Crunchy from OperatorHub, create a 3-node HA cluster from a YAML CR via GitOps, kill the primary, and show automatic failover and backups to object storage.
2. **Monolith-to-microservices with CDC**: a legacy app writes to Postgres, Debezium streams changes into Kafka (Strimzi), and a new Go microservice consumes them, with the outbox pattern.
3. **Private RAG**: Granite model served with vLLM on OpenShift AI, pgvector for retrieval, and a simple chat UI (your front-end strength).
4. **Ansible**: a playbook that provisions RHEL VMs, installs and hardens Postgres, and registers monitoring, with drift remediation.
5. **VM plus container side by side**: a legacy VM database in OpenShift Virtualization accessed by a containerised Go service.

## Technical prep checklist

- [ ] Cloud-native track complete, plus Replication, Consensus (etcd and Raft) and the Global app design
- [ ] Comfortable with `oc`/`kubectl`: deployments, services, routes, PVCs, operators, RBAC, debugging pods
- [ ] Can explain the OpenShift architecture: control plane, etcd, operators everywhere, SCCs, routes vs ingress
- [ ] Hands-on in the Developer Sandbox: deployed an app plus database, used an operator
- [ ] Podman and container fundamentals; RHEL basics (SELinux, systemd, dnf)
- [ ] Ansible basics: inventories, playbooks, roles, collections
- [ ] Kafka, Strimzi and Debezium concepts
- [ ] OpenShift AI and vLLM basics
- [ ] Certifications: EX188 (Containers) or EX280 (OpenShift Admin); RHCSA if you're light on Linux
- [ ] Know the open source story: upstream projects for each product, and licences

## Sample interview questions

1. "Explain what an Operator is, and how it differs from a Helm chart."
2. "A customer wants to run Postgres on OpenShift across two data centres with RPO=0. Design it."
3. "How would you migrate 500 VMs from VMware with minimal disruption?"
4. "Describe how etcd works and why it needs an odd number of members."
5. "What's the difference between Fedora, CentOS Stream and RHEL?"
6. "Present a hybrid cloud strategy to a bank's CIO."

> **SA angle:** Red Hat culture emphasises **open collaboration and transparency**. In interviews, show curiosity, credit communities, and avoid hard-selling. A great signal is having contributed to (or at least engaged with) an open source project. Even a docs PR to Strimzi, CloudNativePG or Debezium shows you get it. Your Go skills make code contributions realistic.

```quiz
Q: "Why pay for OpenShift when upstream Kubernetes is free?" What's the strongest answer?
+ Kubernetes is a core component; OpenShift is an integrated, secured, supported platform (CI/CD, registry, monitoring, security, certified operators, lifecycle) that's consistent across environments, lowering TCO and risk
- Upstream Kubernetes doesn't work in production
- OpenShift is faster at running containers
- Kubernetes is closed source
? Position total cost of ownership, integration, security and support, not the raw scheduler.

Q: What's the difference between an Operator and a Helm chart?
- They're identical
+ Helm templates and installs resources once; an Operator runs a continuous reconciliation loop that manages day-2 operations like failover, backups and upgrades
- Operators only work on OpenShift
- Helm charts can't install databases
? Helm handles packaging and install. Operators encode ongoing operational knowledge.
```
