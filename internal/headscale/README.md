# Headscale REST adapter

`module meshconsole`, package `internal/headscale`, standard library only. No config/auth/frontend/SQLite/Docker code. Parent must wrap `Client.Handler()` in authenticated/CSRF-checked middleware for **all** `/api` business routes.

```go
client, err := headscale.New(baseURL, apiKey, publicURL, policyMode)
handler := client.Handler()
err = client.Verify(ctx) // authenticated /api/v1/health, requires databaseConnectivity=true
```

## Supported versions

Each business request reads actual `/version`. Accepts only stable `0.28.x` / `0.29.x` (optional leading `v`); prereleases, dirty builds, latest strings and other minor versions fail closed. Reference contracts verified against v0.28.0 proto and v0.29.0 / v0.29.5 proto / Swagger, not dynamic latest.

- 0.28.x: nodes/users/routes/pre-auth key reads and explicit writes.
- 0.29.x: same, plus real `POST /api/v1/policy/check`.
- Policy write requires explicit `database` config AND 0.29.x, always server check before write. 0.28 database policy remains readonly here because standalone check is unavailable.
- `policyMode` is a deployment assertion, not an API-discovered fact. Headscale remains the final authority and rejects forbidden updates.

## Console routes and response contract

- `GET /api/connection`: `{url, publicUrl, apiKeyConfigured, configured, mode:"live", policyMode, version, connected, capabilities:{read,write,policyCheck,policyWrite,keys}}`. Checks `/version` and authenticated `/api/v1/health`, only true databaseConnectivity marks connected. Any connection/version failure returns sanitized non-401 error rather than claiming connected.
- `GET /api/devices`: `{devices:[...]}`; ids and userIds strings, online nullable, no OS/version invention (`未采集`, platform `Unknown`). expiry/last ISO or null. Tags normalized without `tag:`. available/approved/serving routes arrays preserved. `givenName` preferred display name. Only whitelisted fields; nested preAuthKey, machine/node/disco keys never leave adapter.
- `GET /api/users`: `{users:[{id,name,displayName,email,initial}]}`; `POST /api/users {name,email}` -> `{user}`.
- `POST /api/devices/{id}/rename {name}`, `/tags {tags}`, `/expire {}` -> `{device}`; DELETE -> `{ok:true}`. No ownership reassignment.
- `GET /api/routes`: flattened available routes, `{routes:[{id:nodeId+"|"+cidr,nodeId,cidr,description:"",approved,serving}]}`. Serving is null when upstream field absent; false only when upstream provided list but route not listed.
- `POST /api/routes/{nodeId} {routes:[CIDRs],expectedRoutes:[CIDRs]}`: requires entire approved set and compares existing set before replacing. CIDRs canonical, available route membership required. Returns `{device}`.
- `GET /api/keys`: `{keys:[{id,name:"预授权密钥 #id",prefix:"•••• last4",owner,userId,reusable,expiry,status,tags}]}`. No raw key; <=4 byte abnormal keys fully suppressed. Status expired / single-use used / valid derived from fields.
- `POST /api/keys {userId,name,ttlDays:1|7|30,reusable}` -> `{key,command,id,expiry}`. `name` validated but **not persisted or sent to Headscale**; no false auxiliary metadata storage. This authenticated no-store create response is the only raw key disclosure. Enrollment command uses public HTTPS URL with shell-quoted arguments, never Docker host.
- `POST /api/keys/{id}/expire {}` -> `{ok:true}`. Both supported minor versions use upstream `{id:string}`.
- `GET /api/policy` -> `{policy,revision:SHA256,mode,writable}`.
- `POST /api/policy/check {policy}` -> `{valid:true}` only after Headscale semantic check. HuJSON stays raw; no incomplete local parser or comment-dropping rewrite.
- `PUT /api/policy {policy,revision}` -> updated policy/revision. Compares current hash, checks upstream, compares again, writes.

## Safety

- Fixed configured origin-only HTTP(S) base URL, no browser-selectable destination/path. Base paths, URL credentials/query/fragment rejected. Public enrollment URL is separate, public HTTPS hostname/IP (no localhost/Docker single-label/.internal/private IP).
- Isolated transport: no environment proxy inheritance; redirects disabled; 8 sec timeout, context propagated, response and incoming bodies bounded 4 MiB.
- All upstream request headers use server API Key. No logs or upstream error bodies exposed. Upstream 401/403 -> `502 headscale_unauthorized`, avoiding invalidating console session.
- static Chinese errors: `headscale_unavailable`, `headscale_unauthorized`, `unsupported_version`, `validation_error`, `conflict`, `policy_read_only`.
- Strict incoming JSON (unknown/trailing fields rejected), numeric uint64 ID strings, names/tags/email/CIDR validation.
- Local mutex serializes policy/routes changes in one adapter. **Headscale REST has no conditional CAS contract**: hash/set comparison reduces conflict risk but cannot guarantee atomic protection against another console/CLI changing data between compare and write. Avoid parallel external writers; distributed revision storage alone cannot fix missing upstream CAS.
- No capability implies arbitrary proxy / Docker socket / config-file mutation. TLS verification remains enabled.

## Evidence

```sh
go test ./internal/headscale -count=1
go test -race ./internal/headscale -count=1
```

Standard tests pass. The Windows environment currently has `CGO_ENABLED=0`; `go test -race` reports `-race requires cgo`, so race execution is not claimed as passed.

Mocks verify exact upstream paths/headers/JSON, uint64 normalization and deep stripping, version gates, redirects and env proxy, route conflicts, policy compare/check/compare/write ordering, key field contracts, no secret propagation, timeout context and size cap.

Official reference URLs:
- https://raw.githubusercontent.com/juanfont/headscale/v0.29.5/gen/openapiv2/headscale/v1/headscale.swagger.json
- https://raw.githubusercontent.com/juanfont/headscale/v0.28.0/proto/headscale/v1/headscale.proto
- https://raw.githubusercontent.com/juanfont/headscale/v0.28.0/proto/headscale/v1/preauthkey.proto
- https://raw.githubusercontent.com/juanfont/headscale/v0.28.0/proto/headscale/v1/node.proto
- https://raw.githubusercontent.com/juanfont/headscale/v0.29.0/proto/headscale/v1/headscale.proto
- https://raw.githubusercontent.com/juanfont/headscale/v0.29.5/hscontrol/handlers.go

No live server or real credentials queried during implementation. Integration and deployment tests are the parent's responsibility.
