# SIS #1 — Blog Platform (posts & comments): system design

**Backend for Highloaded Environment · KBTU · Fall 2026 · Yerakairzhan**

The design is drawn in [`blog-platform-system-design.excalidraw`](blog-platform-system-design.excalidraw)
(open it at [excalidraw.com](https://excalidraw.com) → Open). This document explains every decision on
the board, end to end, in the order I would defend it: requirements → numbers → entities → API →
components → flows → scaling → caching → failures → trade-offs.

**Stack:** Go (grpc-go, grpc-gateway, pgx) · PostgreSQL 16 · Redis · S3 + CDN · Kubernetes.

---

## 1. Requirements

### Functional

1. A user can create, edit and delete a post (title, text, tags).
2. Anyone can read a post with its like and comment counts.
3. A user can see a feed: latest posts, posts by tag, posts by author.
4. A user can comment on a post and reply to a comment.
5. A user can like and unlike a post.

**Out of scope:** sign-up and login (an auth provider issues the JWT; we only verify it), search,
notifications, recommendations, a personal "following" feed.

### Non-functional

| # | Requirement | Why this number |
|---|---|---|
| 1 | 20M DAU; per user per day 10 post views + 5 feed pages; peak = 3× average | A medium-large blog platform. 3× is the usual daily-rhythm peak factor from lecture 1 |
| 2 | ~20K read RPS, ~500 write RPS at peak | Derived in §2. The 40 : 1 ratio decides the whole design: **read-heavy** |
| 3 | p99: read post < 100 ms, feed < 200 ms, write < 300 ms (server side) | Reading must feel instant; a write can afford a sync DB commit + replication |
| 4 | Availability 99.95 %; posts and comments are never lost | 99.95 % = 21.9 min of downtime a month. "Never lost" = RPO 0 for acknowledged writes |
| 5 | Counters and feeds may lag up to ~1 min | Nobody notices a like count that is 30 s old. This permits caching and batching |
| 6 | Keep data 5+ years | Blogs are permanent; storage is sized for 5 years |

**CAP choice:** availability over consistency for everything derived (feeds, counters, cached pages);
strong consistency only where it matters — the primary database row of a post, comment and like.

---

## 2. Capacity estimates

Lecture-1 recipe: traffic → peak → data volume → working set → servers. `avg RPS = per day / 86 400`.

**Reads**

```
20M users x (10 posts + 10 comment pages + 5 feeds) = 500M reads/day
500M / 86 400 = ~6K RPS average  -> x3 = ~18K peak  -> design for 20K
```

Every post view loads the post **and** its first comment page, so a view is 2 reads.

**Writes**

```
posts 200K (1 % of DAU) + comments 2M (0.1/user) + likes 10M (0.5/user) = ~12M writes/day
12M / 86 400 = ~150 RPS average -> x3 = ~450 peak -> design for 500
read : write = 500M : 12M = ~40 : 1
```

**Storage**

| Row | Size | Why | Per day |
|---|---|---|---|
| post | 5 KB | ~10 KB of markdown; PostgreSQL TOAST compresses long text ~2–3×, plus ~1 KB of row and index | 200K × 5 KB = 1 GB |
| comment | 0.5 KB | ~300 B of text + row header + index entries | 2M × 0.5 KB = 1 GB |
| like | 100 B | two `bigint` (16 B) + tuple header (~24 B) + primary-key index entry | 10M × 100 B = 1 GB |

`3 GB/day → ~1.1 TB/year → ~5.5 TB in 5 years`, ×3 copies (primary + 2 replicas) ≈ 16.5 TB of disk.
Per database: posts DB (post + like) grows ~0.7 TB/year, comments DB ~0.4 TB/year. Images are **not** in
the database — they go to S3 behind the CDN.

**Hot data (what Redis must hold)**

```
posts of the last 7 days: 200K x 7 = 1.4M (+ ~100K popular older ones) = 1.5M x 12 KB = ~18 GB
first comment pages of those posts: 1.5M x ~10 comments x 0.5 KB         = ~8 GB
                                                                  total ~26 GB -> ~30 GB Redis cluster
```

The cached post is ~12 KB because it is uncompressed JSON (vs 5 KB compressed in PostgreSQL).
Redis load is ~8K origin RPS × ~2 commands = ~16K ops/s, while one Redis node does ~100K ops/s:
**Redis is sized by memory and failover, not by throughput.**

**Servers**

| Read type | Share of reads | CDN TTL | Expected CDN hit ratio |
|---|---|---|---|
| post page | 40 % | 60 s | ~80 % (hot posts dominate) |
| comment page | 40 % | 5 s | ~40 % |
| feed page | 20 % | 30 s | ~70 % |
| **weighted** | | | **~62 % → "CDN serves ~60 %"** |

```
20K x (1 - 0.6) = ~8K RPS reach the services
Go pod ~2K RPS for cached reads (ASSUMED - to be measured in the SIS #4 load test) -> need 4 pods
run 6 pods per service (2 per AZ): after losing 1 of 3 AZs each service still has 4 pods = 8K RPS
```

12 pods × 2 vCPU = 24 vCPU — cheap insurance. Writes: 500 RPS of single-row inserts is far below what one
PostgreSQL primary on NVMe sustains (thousands of small commits per second), so **one primary per
database is enough**.

**Little's law check (lecture 1):** DB-bound reads after the cache ≈ 10 % of 8K = 800 RPS × ~5 ms =
**4 queries in flight**. A pgx pool of 10–20 connections per pod is plenty; we do not need hundreds of
connections.

---

## 3. Core entities and data model

```
user     id, username, created_at
post     id, author_id, author_name, title, body, tags[], status, like_count, comment_count,
         version, idempotency_key, created_at, updated_at        status: DRAFT -> PUBLISHED -> DELETED
comment  id, post_id, parent_id, author_id, author_name, body, idempotency_key, created_at
like     post_id, user_id                       PRIMARY KEY (post_id, user_id)
```

| Decision | Why | Rejected alternative |
|---|---|---|
| **Snowflake `bigint` ids** (41 bit ms time, 10 bit worker, 12 bit sequence) | Sorted by time → the id itself is the pagination cursor and the "latest" order; new rows append to the end of the B-tree; generated in the Go service with no DB round trip and no central sequence, which matters once we shard | UUIDv4: random → B-tree page splits, 16 bytes, not sortable. `serial`: a single sequence in one DB blocks sharding |
| **`like_count`, `comment_count` stored on the post row** | Reading a post must not run `COUNT(*)` over millions of `like` rows at ~7K RPS. The counts are maintained by the counter worker (§9) | `COUNT(*)` per read — too slow; `UPDATE ... +1` per like — one hot row lock (§9) |
| **`author_name` copied into post and comment** | The post page and comment list can show names without joining `user` (which lives in another DB for comments). The name comes from the JWT at write time | Join per comment, or N extra calls per page. Cost: a renamed user's old content shows the old name until a background job fixes it — acceptable |
| **`version` (optimistic locking)** | Two editors of one post: `UPDATE ... WHERE id = $1 AND version = $2`; 0 rows updated → the second editor gets a conflict instead of silently overwriting | Row locks held while a user edits — impossible across HTTP requests |
| **`idempotency_key`, unique per author** | A mobile client retries after a timeout; the key makes the retry return the first result instead of creating a second post or comment | Hoping clients don't retry; dedupe by text (breaks legitimate repeats) |
| **`status` + soft delete** | Comments still reference the post; moderation and undo need the row; queries filter `status = 'PUBLISHED'` | Hard `DELETE` — breaks references, no audit |
| **`parent_id` (null = top level)** | Replies with one column; the client nests the flat, time-ordered list | Nested-set / closure tables — complex, unnecessary for 2-level threads |
| **`like` PK `(post_id, user_id)`** | The database itself guarantees one like per user per post, so double clicks and retries cannot double-count | A separate uniqueness check in code — races |
| **Two databases: posts DB (user, post, like), comments DB (comment)** | Each service owns its data: independent schema changes, scaling and failure (a comment storm does not slow post reads). No query needs a cross-DB join | One DB — simpler, but one blast radius; one DB per table — overkill |
| **Indexes:** `post (author_id, id)`, `GIN (tags)`, `comment (post_id, id)` | Author page, deep tag pages beyond the Redis feed, and comment pages are all index range scans with a cursor | `OFFSET` pagination — page 5 000 scans 100K rows |

**Durability:** besides replication, a daily base backup + continuous WAL archive to S3 give
point-in-time recovery. Replication protects against a dead disk; backups protect against a bad deploy or
an accidental `DELETE`.

---

## 4. API — proto3 / gRPC

```proto
service PostService {
  rpc CreatePost(CreatePostRequest) returns (Post);
  rpc GetPost(GetPostRequest) returns (Post);
  rpc UpdatePost(UpdatePostRequest) returns (Post);
  rpc DeletePost(DeletePostRequest) returns (Empty);
  rpc ListPosts(ListPostsRequest) returns (ListPostsResponse);
  rpc LikePost(LikePostRequest) returns (Empty);
  rpc UnlikePost(LikePostRequest) returns (Empty);
  rpc GetUploadUrl(GetUploadUrlRequest) returns (GetUploadUrlResponse);
}

service CommentService {
  rpc CreateComment(CreateCommentRequest) returns (Comment);
  rpc ListComments(ListCommentsRequest) returns (ListCommentsResponse);
  rpc DeleteComment(DeleteCommentRequest) returns (Empty);
}
```

(The messages are on the board.)

| Decision | Why |
|---|---|
| **proto3 schema first** | One contract generates the Go server stubs, the clients and the REST gateway. Binary encoding is smaller and faster to parse than JSON. Field numbers make evolution safe: adding a field is non-breaking, removed numbers are `reserved` and never reused |
| **gRPC inside, REST/JSON outside via grpc-gateway** | Browsers cannot speak native gRPC, and **CDNs only cache HTTP GETs** — gRPC calls are POSTs. `google.api.http` annotations map `GetPost` to `GET /v1/posts/{id}` and `ListPosts` to `GET /v1/posts?tag=…`, so the CDN can cache them. One proto file is the source of truth for both |
| **`user_id` from the JWT in metadata, never in the request** | A client cannot act as someone else. The gateway verifies the token and passes `user_id` / `username` in gRPC metadata |
| **Cursor + limit on every list** | `next_cursor` = the last Snowflake id; the next query is `WHERE id < $cursor ORDER BY id DESC LIMIT $limit` — constant cost on any page |
| **`idempotency_key` in create requests** | Safe client retries (see §3) |
| **`version` in `UpdatePostRequest`** | Optimistic locking (see §3) |
| **`LikePost` and `UnlikePost` instead of a toggle** | Both are idempotent: retrying "like" twice still leaves one like. A toggle retried twice flips back |
| **`oneof filter { author_id, tag }` in `ListPostsRequest`** | One list endpoint for 3 feeds: no filter = latest, `tag`, or `author_id` |
| **`ListPosts` returns a 300-character excerpt in `body`** | A feed page of 20 full posts would be ~240 KB; the full body is fetched by `GetPost` |
| **`GetUploadUrl`** | Returns a presigned S3 PUT URL; image bytes never pass through our pods (§5) |
| **No `liked_by_me` in `Post`** | The CDN caches one `Post` for everyone, so it cannot contain per-user data. If needed, a small private call `GetMyLikes(post_ids)` would be added |

**Errors** (gRPC code → HTTP via grpc-gateway): `INVALID_ARGUMENT` 400, `UNAUTHENTICATED` 401,
`PERMISSION_DENIED` 403 (editing someone else's post), `NOT_FOUND` 404, `ABORTED` 409 (version
conflict), `RESOURCE_EXHAUSTED` 429 (rate limit), `UNAVAILABLE` 503 (failover — retry with the same key).

**Deadlines:** the gateway sets a 1 s deadline; Go's `context` carries it through every gRPC, Redis
(20 ms timeout) and PostgreSQL (200 ms timeout) call, so a slow dependency fails fast instead of piling up
goroutines.

---

## 5. Components: what each part is responsible for

| Part | Responsible for | Why it exists / why this choice |
|---|---|---|
| **Client app** (web, mobile) | Calls the REST API; uploads images straight to S3; shows its own new comment immediately from the `CreateComment` response | Showing your own write from the response hides CDN and replica lag from the author |
| **CDN** | Images; caches public GETs: post 60 s, feed 30 s, comment page 5 s; collapses identical requests; purged on delete | Absorbs ~60 % of reads, serves users from a nearby PoP (Almaty ↔ Frankfurt is ~100 ms round trip), and image bandwidth never touches our servers |
| **API gateway** (Envoy + grpc-gateway) | Load balancing, TLS, JWT verification, per-user rate limit (local token bucket per pod), REST → gRPC, routing, the 1 s deadline | Auth and limits are done once, at the edge, not in every service. A local token bucket needs no extra dependency |
| **Post service** (Go) | Posts, feeds, likes, upload URLs; owns the posts DB; cache-aside for `post:{id}`; maintains the tag feed sorted sets; increments like counters in Redis | Read-heavy, CDN-friendly workload. Likes live here because they are keyed by `post_id` and stored in the posts DB |
| **Comment service** (Go) | Comments and replies; owns the comments DB; caches the first comment page; increments comment counters; clears the page cache on a new comment; checks the post exists via `PostService.GetPost` (cached) | Different freshness (5 s, not 60 s) and bursty writes → scales and fails independently. If it is down, the post page still renders without comments |
| **Posts DB** (PostgreSQL, primary + 2 replicas, Patroni) | Source of truth for `user`, `post`, `like` | Relational data, transactions for idempotency and optimistic locking, mature replication and failover. 500 writes/s and a few TB need nothing more exotic |
| **Comments DB** (PostgreSQL, primary + 2 replicas, Patroni) | Source of truth for `comment` | Same reasons; separate for ownership and blast radius |
| **Redis cluster** (~30 GB, primaries + replicas) | Cache (`post:{id}`, `comments:{post_id}`, `feed:{tag}`) and counter deltas | In-memory reads in ~1 ms; data structures we need: sorted sets for feeds, atomic `INCR` / `GETDEL` for counters. Not a source of truth — everything can be rebuilt from PostgreSQL |
| **Counter worker** (Go, 2 replicas) | Every 5 s moves the counter deltas from Redis into `post.like_count` / `comment_count` in one batched transaction | Turns thousands of updates of one hot row into one update per post per 5 s (§9) |
| **S3** | Image storage (originals); the CDN serves them | 11 nines of durability, cheap, and the presigned upload keeps bytes off our pods |
| **Kubernetes** | Runs the stateless pods across 3 AZs; HPA autoscaling on CPU; rolling deploys | Week 13–14 topics; needed for "kill a pod, nothing breaks" |

**Why not more services** (user, like, feed, media, notification) **and why no Kafka:** at this size each
extra service adds network hops and operations cost without having a different scaling profile. Likes share
the posts DB and the `post_id` key; the feed is a query over posts; media is just a presigned URL. The only
asynchronous work is counting, and Redis + a worker does it at 500 writes/s. Kafka would be added later for
notifications or search indexing — both out of scope.

**Why Go:** a goroutine per request is cheap (KB of stack, not MB), so high concurrency needs little memory;
small static binaries start fast, which suits autoscaling; grpc-go and pgx are first-class; `context`
deadlines and `singleflight` (golang.org/x/sync) are in the standard toolbox.

---

## 6. End-to-end flows

### Read a post — `GetPost` (`GET /v1/posts/{id}`)

1. **CDN hit (~80 %)** → returned from the edge. Done.
2. Miss → API gateway (JWT optional for public reads, rate limit) → post service.
3. In-memory cache (only for hot keys, 1 s) → hit → return.
4. Redis `GET post:{id}` (~1 ms) → hit (~98 % at origin) → return.
5. Miss → `singleflight` (one loader per key per pod) → PostgreSQL **replica** `SELECT … WHERE id = $1`
   (primary-key lookup, ~2–5 ms) → `SET post:{id}` with TTL 1 h → return with `Cache-Control: public,
   s-maxage=60`.

Origin cost ≈ gateway 2 ms + service 2 ms + Redis 1 ms (+ PG 5 ms on a miss): ~10 ms, far inside the
100 ms p99 budget — the slack absorbs GC pauses and queueing at peak.

### Read a feed — `ListPosts(tag, cursor, limit)`

CDN (30 s) → post service → Redis `ZREVRANGEBYSCORE feed:{tag}` below the cursor, 20 ids → `MGET`
the cached posts (misses from a replica) → excerpts + `next_cursor`. The post service keeps each feed
sorted set up to date on publish (`ZADD`, score = Snowflake id, trimmed to the latest 1 000) and on delete
(`ZREM`). Deeper pages and the author filter go to the replica: `WHERE author_id = $1 AND id < $cursor
ORDER BY id DESC LIMIT 20` on index `(author_id, id)`; deep tag pages use the `GIN (tags)` index.

### Write a comment — `CreateComment`

1. Gateway: verify JWT, rate limit (e.g. 5 comments/min per user) → 429 if exceeded.
2. Comment service: check the post exists and is published (`PostService.GetPost`, served from cache) and
   that `parent_id` belongs to the same post.
3. `INSERT INTO comment … ON CONFLICT (author_id, idempotency_key) DO NOTHING RETURNING *` on the
   **primary**. Conflict → this is a retry → return the existing comment.
4. The commit waits for one replica (`synchronous_standby_names = 'ANY 1 (r1, r2)'`) → the comment exists
   on 2 nodes in 2 AZs → **RPO 0**. Cost: ~1–2 ms cross-AZ.
5. `INCR post:{id}:comments` and `SADD dirty_posts {id}` in Redis; `DEL comments:{post_id}`.
6. Return the `Comment`; the client shows it at once. Others see it within ~5 s (the CDN TTL of comment
   pages).

### Like — `LikePost` / `UnlikePost`

`INSERT INTO like (post_id, user_id) ON CONFLICT DO NOTHING` → only if a row was inserted:
`INCR post:{id}:likes`, `SADD dirty_posts {id}`. Unlike = `DELETE` → only if a row was deleted: `DECR`.
A double click or a retry inserts nothing the second time, so nothing is double-counted — the primary key is
the idempotency.

### Edit / delete a post — `UpdatePost` / `DeletePost`

`UPDATE post SET …, version = version + 1 WHERE id = $1 AND author_id = $2 AND version = $3`.
0 rows → wrong author (`PERMISSION_DENIED`) or a newer version exists (`ABORTED`). Success → `DEL post:{id}`
in Redis. The CDN may serve the old copy for up to 60 s — accepted by NFR 5. **Delete** additionally purges
the CDN URL and removes the id from the feed sets, because a deleted post must disappear immediately.

### Upload an image — `GetUploadUrl`

Client → `GetUploadUrl(content_type)` → post service checks the type (jpeg / png / webp), creates a key
`images/{snowflake}.{ext}`, returns a presigned S3 PUT URL (10 min, size-limited) and the public CDN URL →
the client uploads the bytes **directly to S3** → puts the CDN URL into the post body. The CDN fetches the
image from S3 on first view; keys never change, so images are cached for a year.

---

## 7. Scaling choice

| Layer | Scaling type | Why |
|---|---|---|
| Go services, gateway | **Horizontal**: N stateless pods behind the load balancer, HPA on CPU | No session state (JWT), so any pod serves any request and pods are disposable |
| Reads | **Copies**: CDN → Redis → PostgreSQL read replicas | Copies of read data do not conflict; the price is staleness, which NFR 5 allows |
| Writes | **Vertical**: one PostgreSQL primary per DB | 500 RPS fits one node; splitting writes is the expensive, hard-to-undo step (lecture 1: "shard last") |
| Later (~2 TB, ~year 3) | **Sharding** `like` and `comment` by `hash(post_id)` | All likes and comments of a post stay on one shard, so a comment page and "did I like it" remain single-shard queries. The posts DB reaches ~2 TB first, mostly because of `like` rows |

Rejected shard keys: `author_id` (one popular author becomes a hot shard, and a comment page only knows
`post_id`); `created_at` (every new write lands on the newest shard).

---

## 8. Caching — Redis, cache-aside

| Key | Value | TTL / invalidation | Why |
|---|---|---|---|
| `post:{id}` | post JSON | 1 h, `DEL` on update / delete | Posts rarely change; `DEL` makes edits visible; the TTL is a safety net |
| `comments:{post_id}` | first comment page | 30 s, `DEL` on new comment | The first page is what everyone loads; deeper pages are rare and go to the replica |
| `feed:{tag}`, `feed:all` | sorted set of the latest 1 000 post ids | updated on publish / delete | A feed page = one `ZREVRANGE` + one `MGET` |
| `post:{id}:likes`, `post:{id}:comments` | counter **deltas** | no TTL; moved to PostgreSQL every 5 s | §9 |

* **Cache-aside**: read Redis → on a miss read PostgreSQL → fill Redis. Writes go to PostgreSQL first, then
  `DEL` the key. Update-then-delete, never delete-then-update: deleting first lets a concurrent reader put
  the old value back before the update commits.
* **Eviction policy `volatile-lru`**: only keys with a TTL (the cache) can be evicted. Counter deltas have
  no TTL, so memory pressure can never silently drop a like.
* **CDN in front**: post 60 s, feed 30 s, comment page 5 s; purge on delete.
* **Hot post** (one post at 50K RPS): the CDN collapses identical requests into one origin fetch per PoP;
  each pod keeps that key in memory for 1 s; `singleflight` lets only one request per pod go to Redis or
  PostgreSQL.
* **Stampede** (a popular key expires and 1 000 requests miss at once): `singleflight` + TTL jitter (±10 %)
  so keys cached together do not expire together.

---

## 9. Counters — the hot-row problem

The naive design runs `UPDATE post SET like_count = like_count + 1` per like. A viral post receiving
2 000 likes/s means 2 000 updates/s on **one row**, each waiting for the previous row lock. This is Amdahl's
serial fraction in production (lecture 1): adding pods does not help, they only queue.

The design instead:

1. The service increments a **delta** in Redis (`INCR post:{id}:likes`) and marks the post dirty
   (`SADD dirty_posts`).
2. Every 5 s the counter worker takes dirty ids (`SPOP`), reads-and-resets each delta (`GETDEL`), and runs
   one transaction: `UPDATE post SET like_count = like_count + $delta …` for the batch, then `DEL post:{id}`
   so the next read shows the new count.

Result: one update per post per 5 s instead of 2 000 per second; the 2 000 `like` inserts/s hit different
rows and do not contend.

**Why deltas, not totals:** if Redis loses a key, an absolute `INCR` would restart at 1 and the worker
would overwrite the real count with 1. With deltas, losing Redis loses at most ~5 s of increments — never
the stored total.

**Accepted trade-off:** a Redis crash can lose up to ~5 s of counter increments. The `like` and `comment`
tables still hold every row, so a nightly job recounts the posts touched that day. Counts are allowed to lag
(NFR 5); the rows themselves are never lost.

---

## 10. Failure modes

| # | Failure | What happens | Mitigation |
|---|---|---|---|
| 1 | Viral post (hot key) | 50K RPS on one Redis key | CDN request collapsing + 1 s in-memory cache + `singleflight` |
| 2 | Cache stampede | A popular key expires; everyone misses at once | `singleflight`, TTL jitter |
| 3 | Redis down | Every read misses | Reads fall back to PostgreSQL replicas (sized for it: ~8K RPS of primary-key lookups over 2 replicas per DB); the gateway tightens rate limits to protect them |
| 4 | PostgreSQL primary down | Writes to that DB fail | Patroni (leader lease in etcd) promotes the sync replica in ~30 s; writes return 503; clients retry with the **same** `idempotency_key`, so a write that did commit is not duplicated. Reads keep working (cache + replicas) |
| 5 | Replica lag | An author reloads and misses their own comment | For 5 s after a write, that user's reads skip the CDN and go to the primary (a short-lived cookie set by the write response); the client also shows its own write from the response |
| 6 | Redis loses counters | Up to ~5 s of like / comment increments | Totals in PostgreSQL are untouched (deltas); nightly recount from `like` / `comment` |
| 7 | Slow DB (lock, vacuum, bad plan) | Requests pile up: by Little's law 800 RPS × 2 s = 1 600 in flight instead of 4 | Timeouts from the deadline, circuit breaker around the DB, max 2 retries with backoff + jitter; the post page renders without comments if the comment service is failing |
| 8 | One AZ lost | A third of pods and DB nodes | 6 pods per service, 2 per AZ — the remaining 4 carry the peak; DB replicas sit in other AZs; Patroni fails over |

---

## 11. Trade-offs I accepted on purpose

| Accepted | In exchange for |
|---|---|
| Counts and feeds up to ~1 min stale | CDN caching and batched counters — the main reason reads are cheap |
| Up to ~5 s of counter increments lost on a Redis crash | No hot-row lock; fixed by a nightly recount |
| An edited post can be served old for up to 60 s | CDN offload; deletes are purged immediately |
| `author_name` copies may be stale after a rename | No cross-DB joins on every page |
| No Kafka, no extra services | Fewer moving parts; added when a real async need appears |

## 12. What changes at 10× (200M DAU)

200K read RPS: more CDN, ~40 pods, more Redis shards — the same shape. 5K write RPS: shard `like` and
`comment` by `hash(post_id)` (planned in §7); introduce Kafka for counter events and notifications. The API
does not change, because nothing in it depends on where rows are stored.

---

## 13. Defense Q&A

**Why is the system read-optimized?** 500M reads vs 12M writes a day (40 : 1). Everything that makes reads
cheap — CDN, Redis, replicas — pays off 40 times more than anything that makes writes cheap.

**Why gRPC if clients use REST?** Inside: a typed contract, binary payloads, HTTP/2 multiplexing, deadlines.
Outside: browsers need REST, and only GETs are CDN-cacheable. grpc-gateway gives both from one proto file.

**How do you prevent duplicate posts or comments?** `idempotency_key`, unique per author, checked with
`ON CONFLICT` in the same insert. A retry returns the original row.

**How do you prevent double likes?** `PRIMARY KEY (post_id, user_id)` + `ON CONFLICT DO NOTHING`; the
counter is incremented only when a row was really inserted.

**Two people edit the same post at once?** Optimistic locking with `version`; the second save gets 409 and
reloads.

**Why not `COUNT(*)` for likes?** ~7K post reads/s × counting millions of rows. The count is precomputed on
the row and kept fresh by the worker.

**What if the primary dies right after `COMMIT` but before the response?** The commit was replicated
synchronously, so the promoted replica has it. The client times out, retries with the same key, and gets
the existing row.

**Why two databases?** Ownership and blast radius: each service owns its schema; a burst of comments cannot
slow down post reads. No feature needs a join across them.

**Why PostgreSQL and not Cassandra or MongoDB?** Writes are only 500/s and the data is a few TB; we want
transactions (idempotent insert, optimistic locking) and secondary indexes. Cassandra pays off at write
rates 100× higher.

**How does a viral post not kill Redis?** The CDN collapses requests per PoP; each pod holds the key in
memory for 1 s; `singleflight` lets one request per pod through. Redis sees a handful of requests per second
for that key.

**How are replies shown?** `ListComments` returns a flat, time-ordered list with `parent_id`; the client
nests it. For very large threads, a `parent_id` filter would be added to `ListCommentsRequest` as a new
field — a non-breaking proto change.

**Where would this break first?** The posts DB size (`like` rows) around year 3 → shard `like` by
`hash(post_id)`. Before that, the measured per-pod RPS (SIS #4) may change the pod count.

---

## AI usage (Generative AI — Level D)

Claude Code (Anthropic) was used to draft the capacity estimates, generate the Excalidraw board and write
this document. The scope, the stack (Go + gRPC/proto3) and the level of detail of the board were decided by
me.
