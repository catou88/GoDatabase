# Experiment HTTP API

The Go HTTP server exposes a small REST-style API for reproducible
data-structure experiments. The server depends on injected experiment runners
and complexity catalogs; it does not expose B+Tree pages, storage handles, or
other internal types.

## Endpoints

### `GET /api/v1/structures`

Lists the supported structures, operations, and theoretical complexity
metadata.

### `POST /api/v1/experiments`

Runs a bounded workload. Requests contain a structure, a non-negative seed,
dataset size, and an operation sequence:

```json
{
  "structure": "btree-memory",
  "dataset_size": 100,
  "seed": 42,
  "trace": true,
  "operations": [
    {"name": "set", "key": "alpha", "value": "one"},
    {"name": "get", "key": "alpha"},
    {"name": "range", "start": "a", "end": "z"}
  ]
}
```

Responses contain operation results, theoretical complexity metadata, measured
timings and allocation fields when collected, and optional structured trace
events. The seed and request sequence are part of reproducibility; the server
must not use unbounded input or expose internal error text.

Invalid JSON, unsupported operations, invalid bounds, and resource-limit
violations return a stable public error code such as `invalid_request`.
Experiment failures return `experiment_failed` without revealing disk paths,
page identifiers, checksums, or stack traces. Detailed failures belong in
server-side logs and metrics.

## CORS and deployment

CORS is unnecessary when the frontend and API share an origin. It is needed
when a separately hosted frontend calls the API from a browser, such as a
static frontend on a different AWS domain. Configure one explicit allowed
origin at deployment time; do not use `*` when credentials or private data are
introduced. TLS termination and authentication belong at the deployment edge
or in a later API layer, not in the experiment request model.
