# AlgoDB Lab Frontend

This is a Next.js client for the Go experiment API.

## Local development

Start the Go API on `http://localhost:8080`, then serve this directory. The
API process must register `server.New(...)` with an injected experiment runner.
The frontend does not start or own that process.

```sh
cd web
npm install
npm run dev
```

For a custom API origin, copy `.env.example` to `.env.local` and set
`NEXT_PUBLIC_API_BASE_URL`. Next.js embeds `NEXT_PUBLIC_*` values into browser
JavaScript, so this variable must contain only a public URL.

For a production build:

```sh
npm run build
```

The Next.js application can run on Node.js, AWS Amplify, or a container behind
CloudFront. To start a production build locally:

```sh
npm run start
```

Open the Next.js URL shown by the dev server. Set the API base URL in the form if the server
uses another address. The API must allow `http://localhost:4173` through its
CORS configuration when the pages are served separately.

## Deployment

The contents of `web/` are static assets and can be hosted by S3 plus
CloudFront, Netlify, or another static hosting service. Deploy the Go API
separately and configure its single allowed browser origin to the deployed
frontend URL. Do not use wildcard CORS for authenticated or private data.
