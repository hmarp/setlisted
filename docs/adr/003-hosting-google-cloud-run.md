# ADR-003: Host on Google Cloud Run

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

The app is a single Go service that serves both the JSON API and the static UI (ADR-001, ADR-002). It stores no user data and has no database (spec GEN-5). Hosting must:

- cost **£0/month as the target**, and a few pounds a month at most (intent, Constraints);
- provide **HTTPS**, because Spotify only accepts HTTPS redirect URLs outside local development (`127.0.0.1`);
- tolerate very low, bursty traffic (a handful of users, a few playlists a month). A cold start after idle is acceptable, but shorter is better (intent, Constraints);
- need as little ops work as possible (no OS patching or certificate management).

## Decision

- Package the Go service as a **container image** and run it on **Google Cloud Run**, scaling to zero when idle.
- Use the default **`*.run.app` HTTPS address**. A custom domain is optional and out of scope for the MVP.
- **Cost guards:**
  - a Google Cloud **budget alert at £1/month**. Alerts warn and don't stop spending, hence the next guard;
  - Cloud Run **maximum instances = 1**, a hard ceiling on compute even under unexpected traffic. One instance is ample for the expected users;
  - **minimum instances = 0**, so nothing is billed while idle.
- The **region** is chosen at the plan stage, after confirming how the always-free allowance applies per region. The trade-off is a US region (most generous free allowance) against a European one (e.g. `europe-west2`, closer to the users). At our traffic, either is expected to cost effectively nothing.
- The service must be **stateless**: any instance can be stopped at any time, so nothing (including sessions) may depend on server memory surviving between requests. How sessions meet this is decided in a separate ADR.
- The deployment mechanism (manual `gcloud` deploy, Cloud Build, or CI) and container build details are left to the plan.

## Alternatives considered

- **Render (free web service).** No card needed and simple git deploys, but it sleeps after 15 minutes idle and takes about a minute to wake. Its plans and limits also changed in 2026.
- **Koyeb (free instance).** Card-free, and it runs the container with short cold starts, but the free tier is modest and its terms are less stable.
- **Azure Container Apps.** Its free allowance and model are equivalent to Cloud Run's. There's no existing Azure setup that would make it the more convenient option, so we went with Cloud Run.
- **Fly.io.** Very good for Go, but there's no free tier for new accounts (about $2–5/month).
- **Railway.** No free tier (about $5/month).
- **AWS Lambda + Function URL** (and Vercel/Netlify functions). Free, but a Lambda isn't an HTTP server. The standard `net/http` app would need a Lambda adapter dependency and would run differently locally and in production, which goes against ADR-001's standard-library-first rule and ties the code to one host.
- **AWS App Runner, ECS Fargate, EC2.** No lasting free tier. Fargate realistically also needs a paid load balancer.
- **Cloudflare Workers/Containers.** Workers don't run standard Go, and Containers need a paid plan.
- **Free VM (Oracle Always Free) or a home server with Cloudflare Tunnel.** Free, but we'd be responsible for the OS, TLS and patching, and for home uptime in the second case.

## Consequences

- Effectively free at our scale, with a hard cap on the worst case.
- A card has to be on file with Google Cloud.
- Cold starts of roughly a second after idle, which is within the intent's tolerance.
- The same container runs locally, in tests and in production, with no code specific to the host. It can move to another container host (Azure Container Apps, Koyeb, Fly, App Runner, a VM) without code changes.
- Statelessness becomes a hard rule for the code, notably for the session design.
- New things for the owner to learn: Google Cloud projects, IAM, Artifact Registry and Cloud Run deployment. This is welcome given the learning goal.
- Logs go to Google Cloud Logging. Logs must not contain user data or tokens, consistent with GEN-5.
