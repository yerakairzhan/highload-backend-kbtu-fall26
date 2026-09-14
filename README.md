# Backend for Highloaded Environment

**KBTU · School of IT and Engineering · Fall 2026 · 3 credits · 15 weeks**

Weekly practice work, measurement reports and course materials for *Backend for Highloaded
Environment*. Each week is a self-contained project on its own branch; `main` is the index and
holds the lecture and practice materials.

| | |
|---|---|
| **Student** | Yerakairzhan — Telegram [@erakairzhan](https://t.me/erakairzhan) |
| **Tutor** | Nurov Arnur — Telegram [@zamimaru](https://t.me/zamimaru) |
| **Format** | 2 h lecture + 1 h practice per week · 4 SIS assignments · oral ticket-based final |
| **Implementation language** | **Go** (see [Stack](#stack)) |

---

## Contents

- [What the course trains](#what-the-course-trains)
- [Stack](#stack)
- [Course calendar and my progress](#course-calendar-and-my-progress)
- [Materials](#materials)
- [Assessment](#assessment)
- [How work is judged](#how-work-is-judged)
- [Using this repository](#using-this-repository)

---

## What the course trains

> Engineering judgement: choosing a protocol, a storage model and a scaling strategy from
> requirements for latency, throughput, consistency and cost — and defending that choice with
> evidence. — *Syllabus, course aims*

The course is about **mechanisms and measurement**, not any single framework: how TCP/TLS,
HTTP/2, HTTP/3 and gRPC behave; how indexes, transactions, MVCC, replication and sharding work;
how caching, queues, distributed algorithms and resilience patterns are built — and how each is
measured in production. A practice task is accepted only with **a working result and numbers**,
never code alone.

Four things Week 1 says you should already own, and every later week builds on:

| Idea | One line |
|---|---|
| Latency ≠ throughput | Throughput can still rise while latency has already broken the budget. |
| The tail is the product | p95 / p99, never the mean. Fan-out turns a rare slow call into a common one (1 − 0.99¹⁰⁰ = 63 %). |
| Little's Law: L = λ × W | Concurrency = arrival rate × time in system. Size pools from this, not by feel. |
| A design needs a number | Target RPS, target p99, data volume, cost ceiling — otherwise there is nothing to defend. |

## Stack

The syllabus's reference stack is **Java 21 / Spring Boot**, PostgreSQL, Redis, Kafka,
Elasticsearch and Docker. The syllabus itself states that *"the emphasis is not on any single
framework but on principles"*, so this repository implements every week in **Go** and keeps the
rest of the reference stack. Each week's README states the deviation and what it changes.

| Layer | Course reference | This repository |
|---|---|---|
| Service | Java 21, Spring Boot (Tomcat) | **Go 1.26**, `net/http` — goroutine-per-request, TLS 1.3, ALPN h2 + http/1.1 |
| Edge / HTTP/3 | nginx or Caddy | **Caddy 2.8** (h3 · h2 · http/1.1, reverse proxy) |
| Load generation | k6, wrk, curl, Java `HttpClient` | **k6** (constant-arrival-rate), custom **Go + quic-go** client for HTTP/3 |
| Data | PostgreSQL 16, Redis, Kafka, Elasticsearch | same, in Docker (from Week 5 on) |
| Observability | OpenTelemetry, Prometheus, Grafana, JFR | **Prometheus + Grafana** (provisioned), Go `pprof` in place of JFR |
| Network impairment | `tc netem` | `tc netem` inside Docker containers |
| Runtime | Docker Compose | Docker Compose — every week: `docker compose up -d --build` |

## Course calendar and my progress

Topics from the official syllabus; the last column is this repository.

| Week | Lecture topic | Practice task | This repo |
|:---:|---|---|---|
| 1 | **Highload and types of scaling.** Vertical vs horizontal, stateless vs stateful, reads vs writes, functional decomposition vs sharding, SLA/SLO/SLI, tail latency, back-of-the-envelope estimation, Amdahl and the USL | Baseline service: p50/p95/p99 under fixed load; capacity estimate for a target RPS; one large instance vs several small | ✅ [`week-1`](../../tree/week-1) |
| 2 | **Networking and protocols I.** TCP handshake and congestion control, TLS 1.3, HTTP/1.1 keep-alive, HTTP/2 multiplexing and HPACK, HTTP/3 and QUIC, WebSocket, SSE | Load-test HTTP/1.1 vs HTTP/2 vs HTTP/3; connection pooling and the cost of the TLS handshake; `netem` delay and loss | 🔧 [`week-2`](../../tree/week-2) — rig done, measurements pending |
| 3 | **Networking and protocols II.** REST vs gRPC vs GraphQL, Protobuf and serialization, schema evolution, API versioning, idempotency, timeouts and retries | gRPC service with Protobuf vs REST/JSON by latency and payload size | — |
| 4 | **Concurrency.** Memory model, thread pools, virtual threads, blocking vs reactive, backpressure, contention, false sharing | Benchmark: thread pool vs virtual threads under load; diagnosing contention · **Quiz 1** | — |
| 5 | **Caching.** Cache-aside / write-through / write-behind, Redis, invalidation and TTL, cache stampede, multi-level caches, CDN | Redis + local cache: cache-aside, stampede protection, hit ratio and its effect on p99 | — |
| 6 | **Databases I.** B-trees and indexes, the query planner, transactions, isolation levels, MVCC in PostgreSQL, locking and deadlocks | PostgreSQL `EXPLAIN ANALYZE`, index design, eliminating N+1 and slow queries · **SIS #1 due** | — |
| 7 | **Databases II.** Sync/async replication, read replicas and lag, failover, sharding and partitioning, connection pooling | Read replicas with PgBouncer, table partitioning, observing replication lag | — |
| 8 | **NoSQL and distributed data theory.** CAP and PACELC, key-value / document / wide-column stores, consistency models, quorums | Data modelling for Cassandra/MongoDB against an access pattern · **Midterm** · **SIS #2 due** | — |
| 9 | **Asynchronous architecture.** Queues and brokers, Kafka topics/partitions/consumer groups, delivery semantics, transactional outbox, CDC | Kafka producer/consumer, at-least-once with an idempotent consumer, the outbox pattern | — |
| 10 | **Distributed systems foundations.** Time and clocks, consensus (Raft), leader election, distributed locks, 2PC vs Saga, why exactly-once is an illusion | Distributed lock on Redis/ZooKeeper, saga orchestration, idempotency keys | — |
| 11 | **Scaling, resilience and the edge.** L4/L7 load balancing, service discovery, autoscaling, rate limiting, circuit breaker, bulkhead, graceful degradation, DDoS protection, AuthN/AuthZ at scale | Rate limiter, circuit breaker, bulkhead; Nginx/Envoy as an L7 balancer | — |
| 12 | **Observability and runtime performance.** Metrics, logs, distributed tracing (OpenTelemetry), profiling, GC tuning, SRE practices | OpenTelemetry with Prometheus and Grafana; profiling and GC tuning · **Quiz 2** | — |
| 13 | **Kubernetes I.** Why orchestration; control plane, etcd, scheduler, kubelet; pods, deployments, ReplicaSets; services, ingress, in-cluster discovery; the reconciliation loop | Deploy to Kubernetes (kind/minikube): manifests, ConfigMaps and secrets, service and ingress, rolling update · **SIS #3 due** | — |
| 14 | **Kubernetes for highload II.** Requests/limits and QoS, CPU throttling and p99, probes and graceful shutdown, HPA and cluster autoscaling, rolling / blue-green / canary, stateful workloads, cost of capacity | HPA under a load test: probes and limits, scale-out behaviour, canary rollout · **SIS #4 due** | — |
| 15 | **Data storage and search; synthesis.** LSM-tree vs B-tree, full-text search (Elasticsearch), object storage, time-series and analytical stores; System Design methodology and case review | Elasticsearch and S3-compatible storage; whiteboard design workshop and mock ticket exam · **End-term** | — |
| 16–17 | **Final exam** — oral, ticket-based: two theory questions (10 pts each) + one System Design case (20 pts) | | |

## Materials

Instructor's slides and the official syllabus, kept on `main`:

| | File |
|---|---|
| Syllabus | [`syllabus-fall-2026.pdf`](syllabus-fall-2026.pdf) |
| Lecture 1 — Introduction to highload and types of scaling | [`lectures/lecture-01-highload-and-scaling.pdf`](lectures/lecture-01-highload-and-scaling.pdf) |
| Lecture 2 — Networking and protocols I | [`lectures/lecture-02-networking-and-protocols-1.pdf`](lectures/lecture-02-networking-and-protocols-1.pdf) |
| Practice 2 — Load testing HTTP/1.1 vs HTTP/2 vs HTTP/3 | [`practices/practice-02-http1-vs-http2-vs-http3.pdf`](practices/practice-02-http1-vs-http2-vs-http3.pdf) |

**Reading list** (from the syllabus; books are not redistributed here):

- *Core:* Kleppmann — *Designing Data-Intensive Applications* (ch. 1 for weeks 1–2) · Xu — *System Design Interview*, vol. 1–2 · Petrov — *Database Internals* · Goetz et al. — *Java Concurrency in Practice*
- *Supplementary:* Nygard — *Release It!* (required for week 11) · Beyer et al. — *Site Reliability Engineering* · Narkhede et al. — *Kafka: The Definitive Guide* · Grigorik — *High Performance Browser Networking* (TCP, TLS, HTTP/2 chapters) · Dean & Barroso — *The Tail at Scale* (2013) · RFC 9000 (QUIC), RFC 9114 (HTTP/3)

## Assessment

<details>
<summary><b>Grade composition and rules</b> (official syllabus)</summary>

| Component | Weight | When |
|---|:---:|---|
| Attendance / participation | 10 % | every session |
| SIS — four cumulative assignments, 5 pts each | 20 % | weeks 6, 8, 13, 14 |
| Midterm / End-term (written: design problems, estimation) | 25 % | weeks 8, 15 |
| Quizzes (computer-based) | 5 % | weeks 4, 12 |
| Final exam — oral, ticket-based | 40 % | weeks 16–17 |

**SIS chain** — one system carried through the semester: ① system design document → ② protocol
implementation (gRPC / HTTP/2 / WebSocket vs a REST/JSON baseline) → ③ database tuning with
before/after p95–p99 → ④ scale-out and resilience on Kubernetes with HPA and a load test.

**Rules that fail you:** > 30 % of lessons missed → F · ≤ 29.4 points across the two attestations
→ not admitted to the final · ≤ 9.4 on the final → F · late practice work receives 50 %.

</details>

## How work is judged

The acceptance criteria repeated in every practice deck — and the checklist every branch here is
written against:

- **Percentiles, not averages.** p50, p95, p99. A mean on its own is not an answer.
- **Conditions stated.** Hardware, runtime version, tool versions, concurrency, duration, warm-up.
- **One variable at a time.** A comparison where two things changed proves nothing.
- **Proved, not assumed.** The negotiated protocol, the hit ratio, the query plan — shown, not inferred.
- **Reproducible.** One command in the README that produces the same numbers on another machine.
- **Explained by a mechanism.** Name the bottleneck — "newer is faster" earns nothing.
- **AI disclosed.** *Generative AI — Level D:* permitted and encouraged for code, config, tests, load
  scenarios and debugging; each README says in 2–3 sentences where it was used; the author must be
  able to explain every line, or the work receives 0.

## Using this repository

```bash
git clone https://github.com/yerakairzhan/highload-backend-kbtu-fall26.git
cd highload-backend-kbtu-fall26

git branch -r                 # list weeks
git checkout week-2           # pick one
cat README.md                 # that week's setup, run commands and report
```

- **One branch per week**, branched from `main`. `main` holds only this index and the materials.
- Every week has a `README.md` (how to run, AI disclosure) and, when the task asks for measurements,
  a `REPORT.md` (environment, table of percentiles, mechanism-level explanations).
- Prerequisites for every week: **Go 1.26+** and **Docker Desktop** (Compose v2). Everything else —
  k6, Caddy, Prometheus, Grafana, PostgreSQL, Redis, Kafka, `netem` — runs in containers.
