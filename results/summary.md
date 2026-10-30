### Latency and bytes per request (median of 3 runs)

| scenario | target | p50 ms | p90 ms | p99 ms | p99 range ms | p99.9 ms | sent B/req | received B/req | failed |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|
| get_post | grpc | 0.642 | 1.404 | 3.091 | 2.926–3.590 | 4.868 | 83 | 10271 | 0 |
| get_post | json_h1 | 0.533 | 1.258 | 2.603 | 2.542–3.236 | 4.051 | 469 | 10873 | 0 |
| list_comments | grpc | 0.623 | 1.368 | 2.854 | 2.627–2.967 | 5.182 | 73 | 7384 | 0 |
| list_comments | json_h1 | 0.497 | 0.887 | 2.598 | 2.460–4.250 | 47.622 | 494 | 10424 | 0 |
| create_comment | grpc | 0.546 | 1.072 | 1.990 | 1.894–2.024 | 4.338 | 400 | 399 | 0 |
| create_comment | json_h1 | 0.427 | 0.659 | 1.205 | 1.162–1.299 | 2.687 | 871 | 627 | 0 |

### Message size: protobuf vs JSON (body only, no headers)

| scenario | direction | message | protobuf B | JSON B | JSON / protobuf |
|---|---|---|--:|--:|--:|
| get_post | response | Post | 10258 | 10787 | 1.05× |
| get_post | request | IdRequest | 10 | 27 | 2.70× |
| list_comments | response | CommentList | 7350 | 10325 | 1.40× |
| list_comments | request | CommentListRequest | 12 | 44 | 3.67× |
| create_comment | response | Comment | 360 | 495 | 1.38× |
| create_comment | request | CommentCreate | 351 | 401 | 1.14× |

