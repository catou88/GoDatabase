# Cache and backing-store composition

Version 1 sessions support one optional bounded LRU cache over exactly one
backing structure. Independent structures are comparisons, not layers.
Add "version": 1 and "cache": {"capacity": 32, "policy": "lru"} to a request.
Capacity is 1 to 256 entries. Input key/value limits also bound cache payload.
Setup fills backing storage before creating the empty cache. Repeat Get in one
workload to observe cold misses and warm hits.

Misses cache found values; missing keys are not cached. Set writes through then
invalidates. Delete invalidates after success. Failed writes preserve the cache.
Range bypasses the cache. Each experiment owns its cache and backing store.
Responses include hits, misses, evictions, backing calls and nested backing time.
Total latency includes cache overhead. Backing time must not be added to it.

Redis-, MongoDB-, and MySQL-inspired configurations are educational presets only.
This cache is not Redis. The lab does not reproduce vendor protocols, query
engines, optimizers, consistency guarantees, or production performance.
