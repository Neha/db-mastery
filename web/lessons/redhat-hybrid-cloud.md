## Red Hat in one paragraph

Red Hat (an IBM subsidiary since 2019, operating independently) builds **enterprise open source**. It takes upstream community projects (Linux, Kubernetes, Ansible, Kafka, Debezium, vLLM), hardens, tests and certifies them, and sells **subscriptions** for support, security updates, certifications and lifecycle guarantees. Its strategy is **open hybrid cloud**: one consistent platform across on-prem datacentres, every public cloud and the edge.

> **Key idea:** Red Hat doesn't sell software licences; the code is open. It sells **trust, support and lifecycle** (security patches, 10-year RHEL lifecycles, certified ecosystems). Understanding this business model is essential for the interview.

## The portfolio you must know

| Product | What it is | Upstream |
|---|---|---|
| **RHEL** (Red Hat Enterprise Linux) | The enterprise Linux standard | Fedora / CentOS Stream |
| **OpenShift** | Enterprise Kubernetes platform (developer tools, CI/CD, GitOps, service mesh, serverless, virtualisation) | Kubernetes, OKD |
| OpenShift Virtualization | Run VMs alongside containers (a major VMware migration play) | KubeVirt |
| **Ansible Automation Platform** | IT automation (infrastructure, network, security, DB provisioning) | Ansible |
| **Red Hat AI** (OpenShift AI, RHEL AI, AI Inference Server) | MLOps and GenAI platform: model training, serving (vLLM), hybrid AI | Open Data Hub, Kubeflow, vLLM, InstructLab |
| Streams for Apache Kafka | Kafka on OpenShift | Strimzi |
| Red Hat build of Debezium | Supported CDC | Debezium |
| Application Foundations / runtimes | Quarkus, JBoss EAP, Camel (integration), Apicurio | Various |
| OpenShift Data Foundation | Software-defined storage | Ceph, Rook, NooBaa |
| Advanced Cluster Management / Security | Multi-cluster management, Kubernetes security | Open Cluster Management, StackRox |
| Managed offerings | ROSA (OpenShift on AWS), ARO (Azure), OpenShift Dedicated | — |

**ROSA (Red Hat OpenShift Service on AWS)** is jointly operated by Red Hat and AWS. With your AWS background, you're unusually well placed to discuss ROSA architectures.

## Where databases fit at Red Hat

Red Hat doesn't sell its own database. Instead, it's the **platform** that runs databases and data services:

1. **Databases on OpenShift** via certified operators: Crunchy Data and EDB (Postgres), MongoDB Enterprise, Couchbase, Redis, Cockroach, and others. Red Hat SAs design these architectures with partners.
2. **Data in motion**: Kafka (Streams), Debezium (CDC), Camel (integration), Apicurio (schemas).
3. **AI on your data**: OpenShift AI or RHEL AI, with vector databases (pgvector, Milvus) and data pipelines on OpenShift.
4. **Automation**: Ansible playbooks for database provisioning, patching and compliance across the hybrid estate.
5. **Partners**: Red Hat co-sells with ISVs, including **MongoDB, Databricks-adjacent tools**, IBM (watsonx, Db2) and the cloud providers.

## The hybrid cloud value proposition

Common customer situations Red Hat SAs address:

| Customer pain | Red Hat answer |
|---|---|
| "We're on 3 clouds plus on-prem, and each has different tooling" | OpenShift everywhere: one platform, skills and security model |
| "Regulators require data on-prem or in-country" | Same platform on-prem, sovereign clouds, edge |
| "VMware licensing changes are killing our budget" | OpenShift Virtualization: migrate VMs, then modernise to containers gradually |
| "Our monolith blocks innovation" | Containerise → strangler-fig with Kafka/Debezium → microservices on OpenShift |
| "We want GenAI, but can't send data to public APIs" | Red Hat AI: run open models (Granite, Llama, Mistral) with vLLM on your infrastructure |
| "Manual ops don't scale" | Ansible plus GitOps plus operators |
| "Avoid cloud lock-in" | Open source, portable workloads, multi-cloud |

## Open source fluency (they will test this)

- **Upstream first**: Red Hat contributes fixes upstream before shipping them in products. Know the upstream for each product.
- **Licences**: permissive (Apache 2.0, MIT, BSD) vs copyleft (GPL, AGPL), and "source-available" licences that are *not* OSI open source (SSPL used by MongoDB; BSL used by HashiCorp, among others). Be neutral and factual; Red Hat partners with MongoDB.
- **Community vs enterprise**: Fedora → CentOS Stream → RHEL; OKD → OpenShift; Strimzi → Streams for Apache Kafka.
- **Why customers pay**: support SLAs, security response (CVE patches), certifications (FIPS, Common Criteria), long lifecycles, an ecosystem of certified hardware and software, and indemnification.

## Red Hat SA vs AWS SA: the mindset shift

| AWS SA | Red Hat SA |
|---|---|
| "Use our managed service" | "Run it consistently anywhere, including on AWS (ROSA)" |
| Single cloud depth | Multi-cloud and on-prem breadth |
| Proprietary services | Open source projects and community |
| Consumption billing | Subscriptions (per core, node or socket) |
| Customer is cloud-ready | Customer is often mid-migration with a large legacy estate |

> **SA angle:** Turn your AWS background into a superpower: *"I've helped customers build on AWS. I've also seen where they hit limits: regulatory needs to stay on-prem, multi-cloud mandates, lock-in concerns. OpenShift, including ROSA on AWS, gives them one platform across all of it. I can speak AWS fluently and bridge to the hybrid story."*

> **Hands-on:** In the Developer Sandbox, deploy a Go app plus Postgres with OpenShift's developer console (Import from Git, S2I or a Containerfile). Add a Kafka topic via Strimzi if available, and write a short architecture doc as if for a customer. Bonus: write an Ansible playbook that provisions the Postgres container on a RHEL VM.

```quiz
Q: What does Red Hat primarily sell?
- Proprietary software licences
+ Subscriptions providing support, security updates, certifications and lifecycle for enterprise open source
- Cloud infrastructure
- Database licences
? The code is open source. Customers pay for trust: support, patches, certification and long lifecycles.

Q: What is ROSA?
+ Red Hat OpenShift Service on AWS, jointly managed by Red Hat and AWS
- A Red Hat database
- An Ansible module
- A Kubernetes storage driver
? ROSA is a managed OpenShift service on AWS, a natural bridge for someone with an AWS background.

Q: Which Red Hat offering helps customers move VMs off VMware onto a container platform?
- RHEL AI
+ OpenShift Virtualization (based on KubeVirt)
- Ansible Lightspeed
- Streams for Apache Kafka
? OpenShift Virtualization runs VMs and containers side by side, enabling gradual modernisation.

Q: How does Red Hat typically participate in a customer's database strategy?
- By selling its own proprietary database
+ As the hybrid platform (OpenShift/RHEL) running certified database operators and data-in-motion services, often with partners
- By replacing all databases with Kafka
- It doesn't engage with data workloads
? Red Hat provides the platform, automation and streaming. Databases come via partners and open source projects.
```
