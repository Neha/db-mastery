## Discovery: the most underrated SA skill

Great SAs talk less and ask more in the first meeting. The goal of discovery is to understand **business pain, technical reality and decision process** well enough to propose something the customer will actually buy and adopt.

### A discovery framework (pain → impact → vision → plan)

| Stage | Questions |
|---|---|
| **Current state** | "Walk me through how this works today." "What does the architecture look like?" |
| **Pain** | "What's not working?" "What happened the last time it broke?" |
| **Impact** | "What does that cost you: revenue, time, risk, people?" "Who feels it most?" |
| **Vision** | "If this were solved, what would be different in 12 months?" |
| **Requirements** | Scale, latency, consistency, compliance, integrations, skills |
| **Decision** | "Who else is involved?" "How have you made similar decisions?" "What would a successful evaluation look like?" "Timeline?" |

Techniques:
- **Open questions first**, then narrow down. Avoid yes/no questions early.
- **Quantify**: "How long?", "How many?", "How much does that cost?"
- **Play it back**: "So if I understood correctly, the three biggest issues are…". It builds trust and corrects misunderstandings.
- **Find the champion** and understand what success means *for them personally*.
- **Silence is fine.** Let them fill it.

> **Tip:** Sales methodologies you'll hear: **MEDDPICC** (Metrics, Economic buyer, Decision criteria, Decision process, Paper process, Identify pain, Champion, Competition), common at MongoDB and Databricks. Know what each letter means and how SAs contribute (especially Metrics, Decision criteria and Technical champion).

## Demos that win

A demo isn't a feature tour. It's a **story** that proves the customer's problem can be solved.

### The demo structure (Tell-Show-Tell)

1. **Context (1–2 min)**: "You told us X is painful because Y. Here's how this looks with our platform."
2. **The "wow" first (2–3 min)**: show the end result before the mechanics (the dashboard, the search results, the failover with zero errors).
3. **How it works (5–10 min)**: the 2–3 capabilities that matter *to them*, connected to their pain points.
4. **Recap the value (1 min)**: map what they saw back to their metrics.
5. **Next step**: POC scope, workshop, architecture review.

### Demo hygiene checklist

- [ ] Rehearsed end to end at least 3 times, at 10-minute and 20-minute versions
- [ ] Realistic data in the customer's domain (not "foo/bar")
- [ ] A backup plan: recorded video, screenshots, a second environment
- [ ] Fonts large, notifications off, tabs pre-opened, terminal history clean
- [ ] Narration prepared: what you'll *say* while waiting for things to load
- [ ] Anticipated questions with prepared answers (security, pricing, limits)
- [ ] Code available in a repo to share afterwards

> **SA angle:** Your front-end skills let you build demos with a **real UI** instead of terminals and JSON. A clean React page doing live search-as-you-type on Atlas, or a dashboard fed by a Databricks pipeline, is far more memorable to business stakeholders.

## Whiteboarding with confidence

### Before drawing
- Restate the problem and confirm the requirements (write them in a corner of the board).
- State your assumptions explicitly.

### While drawing
- Draw **left to right**: users/sources → ingestion/API → processing → storage → consumers.
- Use consistent shapes: boxes for services, cylinders for data stores, arrows labelled with *what flows* (events, queries, CDC).
- Start high level (5–7 boxes), then **zoom into the hardest part** when asked (or proactively).
- Narrate the trade-offs: "I'm putting search in Atlas rather than a separate Elasticsearch cluster because it removes the sync pipeline. The trade-off is…"
- Number the request flow (1, 2, 3…) to walk through a scenario.

### Handling curveballs
- "What if traffic grows 100x?" Identify the first bottleneck and how you'd address it.
- "What if this region fails?" Walk the failure path, RPO and RTO.
- "The customer insists on X." Explore *why*, then find a design that satisfies the underlying need.
- "I don't know." That's fine: say how you'd find out, and reason from first principles.

## Mock scenarios to practise

Record yourself (video) running each in 30 minutes:

1. **MongoDB**: "A gaming company's player profiles and leaderboards are on DynamoDB. They struggle with querying and want to add AI-driven matchmaking." Do discovery, then design, then a demo outline.
2. **Databricks**: "An insurer has a Teradata warehouse, Hadoop for data science and SAS models. Costs are rising and AI pilots stall." Do discovery, then a migration architecture, then a business case.
3. **Red Hat**: "A telco runs 3,000 VMs on VMware plus some apps on EKS, and needs edge deployments at 500 sites." Do discovery, then a hybrid architecture, then a phased roadmap.
4. **Any**: "A healthcare startup wants a patient-facing GenAI assistant with HIPAA compliance."

## Handling the POC

A POC is expensive. Scope it tightly:
- **Success criteria agreed in writing** before starting (measurable: latency < X, cost < Y, feature Z works with their data)
- Time-boxed (2–4 weeks), with named customer participants and regular check-ins
- Tied to a decision: "If we meet these criteria, will you move forward?"

```quiz
Q: What's the main purpose of discovery in an SA engagement?
- To present every product feature
+ To understand business pain, technical requirements and the decision process well enough to propose an adoptable solution
- To collect the customer's budget only
- To schedule a demo as fast as possible
? Discovery shapes everything after it. Without it, demos and designs miss the mark.

Q: What should come first in a strong demo?
- Installation steps
- A full feature tour
+ Context tied to the customer's pain, followed quickly by the "wow" end result
- Pricing
? Lead with relevance and outcome, then explain how it works.

Q: Before starting a POC, what's the most important thing to agree?
+ Written, measurable success criteria and the decision that follows if they're met
- The colour scheme of the demo
- Which sales rep attends
- The number of slides
? Clear criteria prevent endless POCs and tie technical success to a business decision.
```
