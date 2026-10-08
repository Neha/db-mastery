## What stays the same

You already have the hardest-to-teach skills:

- **Customer-facing technical leadership**: discovery, whiteboarding, objection handling
- **Architecture breadth**: networking, security, IAM, HA/DR and cost on the largest cloud
- **Working with sales**: account plans, opportunities, technical wins, POCs
- **Front-end background**: you can build demos with a polished UI quickly, and you think in user journeys and developer experience (which MongoDB especially values)

## What changes

| At AWS | At MongoDB / Databricks / Red Hat |
|---|---|
| Breadth across 200+ services | **Depth** in one platform and its domain (data or hybrid platform) |
| Customer often already committed to AWS | You must **win** against AWS-native services (and partner with AWS) |
| "Which AWS service?" | "Why our platform at all, and why now?" |
| Large SA org, many specialists | Smaller teams; you're expected to be hands-on (demos, POCs, code) |
| Consumption-based, broad | Consumption (Atlas, Databricks DBUs) or subscription (Red Hat) with a specific product motion |

> **Key idea:** Your story should be "**I bring cloud architecture breadth and customer leadership, and I've deliberately built deep data expertise**". This portal, your labs and your demo are the evidence.

## Typical SA interview loops

Loops vary by team and level, but commonly include:

| Stage | What's assessed | How to prepare |
|---|---|---|
| Recruiter screen | Motivation, background, logistics | A crisp 2-minute "why this company, why now" |
| Hiring manager | Customer impact, sales collaboration, culture fit | 6–8 STAR stories with metrics |
| Technical deep dive | Database or data fundamentals, platform knowledge | Tracks 1–6 of this portal |
| **Presentation / demo** | Present a solution or demo to a mock customer panel | Build and rehearse one (see the discovery and demo lesson) |
| System design / whiteboard | Architecture for a scenario | Track 8 |
| Peer SA / sales rounds | Collaboration, objection handling, discovery | Mock discovery calls |
| Values / leadership | Company values (MongoDB: "Think Big, Go Far", ownership; Databricks: customer obsession, truth seeking, first principles; Red Hat: open source principles, transparency, collaboration) | Read their culture pages; map stories to the values |

## Your positioning by company

### MongoDB
"I've helped hundreds of AWS customers build apps. Developer velocity and operational simplicity always decide the database choice. I know where DynamoDB and DocumentDB struggle, and I can help AWS customers adopt Atlas on AWS, through Marketplace, burning down their commits."

### Databricks
"I've architected data platforms with Glue, EMR, Redshift, SageMaker and Lake Formation, and seen the pain of stitching them together: copies of data, split governance, BI and ML on different stacks. I want to help customers consolidate on an open lakehouse with unified governance, and I speak AWS fluently, which matters because many Databricks customers run on AWS."

### Red Hat
"I've seen customers hit the limits of single-cloud: regulation, sovereignty, legacy estates, VMware costs. I know AWS deeply, including ROSA, and I can bridge customers to an open hybrid platform where OpenShift runs consistently on-prem and in every cloud."

## Building your evidence (portfolio)

1. **GitHub repo** with your lab solutions from this portal (Bitcask, LSM, consistent hashing, MVCC). It shows depth.
2. **One polished demo** per target company:
   - MongoDB: a Go plus React app with Atlas Search autocomplete and Vector Search RAG.
   - Databricks: a notebook-based medallion pipeline plus an AI/BI dashboard plus a small RAG agent.
   - Red Hat: a Go app plus Postgres (operator) plus Kafka/Debezium outbox on the OpenShift Sandbox.
3. **Certifications**: MongoDB Associate Developer or DBA; Databricks Data Engineer Associate (plus GenAI Engineer Associate); Red Hat EX280 (OpenShift Administration) or EX188 (containers).
4. **A blog post or talk**: "What I learned building an LSM-tree in Go as an SA". This stands out enormously.
5. **LinkedIn**: headline "Solutions Architect | Data platforms, databases & GenAI | AWS"; share lab learnings weekly.

## STAR stories to prepare (map AWS experience)

| Theme | Example prompt |
|---|---|
| Technical win | "Tell me about a deal you won through technical leadership." |
| Migration | "Describe a complex migration you led." (e.g. Oracle → Aurora, on-prem → AWS) |
| Performance rescue | "A customer had a critical performance problem…" |
| Disagreement | "When did you push back on a customer or your sales team?" |
| Failure | "A POC that failed, and what you learned." |
| Learning fast | "When did you master a new technology quickly?" (this portal!) |
| Influence | "Influencing an executive without authority." |
| Product feedback | "When did you bring customer feedback to product teams?" |

Format: **Situation** (2 sentences), **Task** (1), **Action** (most of the time, saying "I", not "we"), **Result** (numbers: revenue, latency, cost, time).

```quiz
Q: What's the strongest narrative for an AWS SA moving to a data-platform SA role?
- "I'm tired of AWS"
+ Cloud architecture breadth and customer leadership, plus deliberately built and demonstrated depth in data
- "I'll learn the product on the job"
- "AWS products are worse than yours"
? Lead with transferable strengths and concrete evidence of new depth (labs, demos, certifications), and stay positive about AWS (a key partner).

Q: In a STAR answer, which part should take most of your time?
- Situation
- Task
+ Action, describing what you specifically did
- Result
? Interviewers assess your behaviours and decisions. Keep the context short and quantify the result.
```
