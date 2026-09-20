# AlgoDB Lab

AlgoDB Lab is an educational database and data-structure laboratory for
comparing implementations, measuring their behavior, and tracing how
operations execute. The repository contains a string-based key-value API,
durable page-backed B+Tree storage, and a Next.js visualizer for following an
operation through the logical table, WAL, buffer pool, B+Tree, and disk layers.

## Usage

```go
package main

import (
	"fmt"
	"log"

	"godatabase/db"
)

func main() {
	database, err := db.Open("app.db")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	if err := database.Set("language", "Go"); err != nil {
		log.Fatal(err)
	}

	value, found, err := database.Get("language")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(value, found)
}
```

`Open` creates or reopens durable storage. The caller owns the returned database
and must call `Close`. `New` remains available for temporary in-memory storage.

See [Durable Storage](docs/durable-storage.md) for API semantics, durability and
recovery guarantees, free-page reuse, compatibility, and known limitations.

See [Profiling AlgoDB Lab](docs/profiling.md) for copy-pasteable CPU, memory,
allocation, and execution-trace workflows.

See [Educational Database Lab Architecture](docs/architecture.md) for current
package boundaries, the experiment API direction, and staged storage/engine
refactoring.

## Run the lab locally

Start the experiment API from the repository root:

```bash
go run ./cmd/lab-server
```

In another terminal, start the React/Next.js frontend:

```bash
cd web
npm install
npm run dev
```

Open <http://localhost:3000>. The API listens on port `8080` and permits both
`localhost:3000` and `127.0.0.1:3000` during local development.

## Deploy on AWS

Deploy the Go API and Next.js frontend as separate services. A practical first
deployment is the Go API on [AWS App Runner](https://docs.aws.amazon.com/apprunner/latest/dg/)
or ECS, and the frontend on [AWS Amplify](https://docs.aws.amazon.com/amplify/latest/userguide/)
or another managed Next.js host. Keep the API private behind an API Gateway or
load balancer when the frontend is public.

Configure the API service with:

```text
ALGODB_ADDR=:8080
ALGODB_FRONTEND_ORIGIN=https://<your-frontend-domain>
```

Configure the frontend at build time with:

```text
NEXT_PUBLIC_API_BASE_URL=https://<your-api-domain>
```

`NEXT_PUBLIC_API_BASE_URL` is included in the browser bundle, so it must contain
only a public HTTPS endpoint, never credentials or private tokens. Restrict
the API CORS origin to the deployed frontend domain, terminate TLS at the
managed AWS edge, and add authentication/rate limiting before exposing the
experiment API publicly. Use an AWS secret manager for any future server-side
credentials.

For production, build and test both services in CI, deploy the API first,
verify `GET /api/v1/structures`, then build the frontend with the API URL and
run a smoke test against the deployed frontend.
