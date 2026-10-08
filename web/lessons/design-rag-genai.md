## Why this matters for every target company

GenAI is the top budget priority across enterprise IT. All three companies sell into it:
- **MongoDB**: Atlas Vector Search plus operational data plus Voyage AI embeddings, "the data layer for AI apps and agents".
- **Databricks**: Mosaic AI, Vector Search, Agent Bricks and MLflow on governed lakehouse data.
- **Red Hat**: Red Hat AI (OpenShift AI, RHEL AI, the vLLM-based AI Inference Server), private and hybrid AI with open models.

Expect a GenAI architecture question in every loop.

## RAG in one picture

**Retrieval-Augmented Generation** grounds an LLM's answer in your data, reducing hallucination and avoiding fine-tuning for knowledge.

```text
INGEST (offline / streaming)
 docs, tickets, PDFs, DB rows ─▶ parse ─▶ chunk ─▶ embed (embedding model) ─▶ store vectors + text + metadata
                                                                                   (vector index)
QUERY (online)
 user question ─▶ (rewrite) ─▶ embed ─▶ vector search (+ keyword, + metadata filters: tenant, ACL, date)
              ─▶ rerank top-k ─▶ build prompt (instructions + retrieved chunks + question)
              ─▶ LLM ─▶ answer with citations ─▶ guardrails ─▶ user
              └──── log traces, feedback, evaluations ────┘
```

## Key concepts

| Concept | What to know |
|---|---|
| **Embedding** | A vector (e.g. 768–3072 floats) capturing meaning; similar text means nearby vectors |
| **Similarity** | Cosine, dot product, Euclidean |
| **ANN index** | Approximate nearest neighbour. **HNSW** (graph-based, fast and accurate, memory hungry) or IVF (clustered). Trades a little recall for big speed gains |
| **Chunking** | Split documents into passages (e.g. 300–800 tokens with overlap), or semantic or structure-aware chunks (by headings) |
| **Hybrid search** | Combine vector (semantic) with keyword (BM25) results, e.g. Reciprocal Rank Fusion. Helps with codes, names and acronyms |
| **Reranking** | A cross-encoder model reorders the top 50 to pick the best 5, a big quality boost |
| **Metadata filtering** | Tenant, permissions, product, date. **Security-critical**: never retrieve what the user can't see |
| **Quantisation** | Store vectors as int8 or binary to cut memory 4–32x with small recall loss |
| **Evaluation** | Retrieval metrics (recall@k, MRR) plus answer metrics (faithfulness, relevance), LLM-as-judge, human feedback |

## Design decisions an SA must drive

1. **Where do vectors live?**
   - *Next to the source data* (Atlas Vector Search, pgvector, Databricks Vector Search synced from Delta): no sync pipeline, one security model, fresh data.
   - *Dedicated vector DB* (Pinecone, Milvus, Weaviate): specialised features and scale, but another system to secure and sync.
2. **Freshness**: how quickly must new or updated content be searchable? Use change streams or CDC for incremental re-embedding.
3. **Access control**: filter by the user's entitlements *at retrieval time*, as a pre-filter in the vector query.
4. **Model hosting**: public API (OpenAI, Anthropic, Gemini, via Bedrock) vs self-hosted open models (Llama, Mistral, Granite on vLLM) for data sovereignty and cost control.
5. **Cost**: embedding volume, index memory, LLM tokens. Cache frequent answers; route simple queries to smaller models.
6. **Evaluation and observability** from day 1: tracing (MLflow Tracing, OpenTelemetry), golden test sets and regression tests on prompts.

## Example: support assistant on each stack

### MongoDB Atlas
```js
// chunks collection: text + embedding + metadata in one document
{ _id, docId, tenantId: "acme", product: "router-x", acl: ["support","partner"],
  text: "To reset the router…", embedding: [/* 1024 floats */], updatedAt }

db.chunks.aggregate([
  { $vectorSearch: { index: "vec", path: "embedding", queryVector: qv, numCandidates: 300, limit: 20,
      filter: { tenantId: "acme", acl: { $in: userGroups } } } },
  // optionally: $unionWith a $search (keyword) branch and fuse ranks → hybrid search
  { $project: { text: 1, docId: 1, score: { $meta: "vectorSearchScore" } } }
])
```
Pitch: operational data, vectors, full-text and metadata in one platform, with change streams triggering re-embedding (Voyage AI models) and Search Nodes scaling independently.

### Databricks
Delta table of chunks → **Vector Search index (auto-synced from Delta)** → an agent built with the Agent Framework or Agent Bricks → **Model Serving** endpoint (foundation model or external model via AI Gateway) → **MLflow** tracing and evaluation → governance and lineage in **Unity Catalog**. Pitch: RAG over all your governed enterprise data, with evaluation built in.

### Red Hat
Documents in object storage (ODF) → ingestion pipelines on **OpenShift AI** (Kubeflow Pipelines) → embeddings and vectors in **pgvector** (CloudNativePG/Crunchy) or Milvus on OpenShift → open models (Granite, Llama) served with **vLLM** (Red Hat AI Inference Server) on GPUs → the same stack on-prem, in the cloud and at the edge. Pitch: private, sovereign, cost-controlled AI on open source, with no data leaving your environment.

## A minimal RAG retrieval in Go (pgvector)

```go
func Retrieve(ctx context.Context, pool *pgxpool.Pool, qv pgvector.Vector, tenant string, k int) ([]Chunk, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, text, 1 - (embedding <=> $1) AS score
		FROM chunks
		WHERE tenant_id = $2              -- security filter BEFORE ranking
		ORDER BY embedding <=> $1         -- cosine distance, HNSW index
		LIMIT $3`, qv, tenant, k)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Chunk])
}
```

## Agents (the next step)

Agents let the LLM **call tools** (search, SQL queries, APIs) in a loop to complete tasks. The data platform supplies tools (governed queries, vector retrieval), memory (conversation and state stored in MongoDB or Lakebase/Postgres) and guardrails. Standards like **MCP** (Model Context Protocol) expose data sources as tools, and all three vendors offer MCP integrations.

## Failure modes to raise

- Irrelevant retrieval, causing confident wrong answers. Fix with hybrid search, reranking, better chunking and evaluation.
- **Data leakage across tenants or permissions.** Enforce filters server-side and test them.
- Stale content: incremental sync and TTLs.
- Prompt injection inside retrieved documents: sanitisation, instruction hierarchy, output guardrails.
- Cost blowups: token budgets, caching, model routing.

```quiz
Q: Why is metadata pre-filtering in vector search a security requirement, not just an optimisation?
+ Without it, chunks the user isn't entitled to see can be retrieved and leaked through the LLM's answer
- It makes embeddings smaller
- LLMs can't read metadata
- It's required by HNSW
? Retrieval must enforce entitlements. Once text is in the prompt, it can leak.

Q: What problem does hybrid search (vector + keyword) solve?
- It removes the need for embeddings
+ Pure vector search can miss exact terms like product codes, names and acronyms; keyword search catches them and fusion combines both
- It reduces LLM token cost to zero
- It replaces reranking
? Semantic and lexical retrieval are complementary. Fusion (e.g. RRF) improves recall and precision.

Q: What's a key argument for storing vectors alongside operational data (e.g. Atlas Vector Search, or Databricks Vector Search synced from Delta)?
+ No separate sync pipeline, consistent security and governance, and fresher data
- Vectors are smaller there
- It eliminates the need for an LLM
- It guarantees zero hallucinations
? Fewer systems means less drift, fewer security gaps and simpler operations.

Q: A bank can't send data to public LLM APIs. Which architecture fits?
- Use a public chatbot with anonymised prompts only
+ Self-host open models (e.g. served with vLLM) on its own infrastructure or private cloud, with the vector store in the same environment
- Skip GenAI entirely
- Email documents to the model provider
? Private, sovereign AI with open models is exactly the Red Hat AI pitch, and it's also possible with Databricks or Atlas deployments in the customer's region and cloud account.
```
