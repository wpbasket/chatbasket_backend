# Heroku Mail Relay

An RPC-to-SMTP gateway. `chatbasket-api` hands it a message over Connect RPC; five
workers drain a 200 slot queue and deliver through Zoho over SMTP. It exists because
the primary infrastructure blocks outbound SMTP ports.

The module has no dependencies beyond `connectrpc.com/connect` and
`google.golang.org/protobuf`, and runs inside a 512 MB dyno.

## Endpoints

| Path | Purpose |
|------|---------|
| `POST /rpc_core_email.v1.EmailService/SendEmail` | The only RPC. Binary protobuf (`application/proto`) over Connect's unary protocol. |
| `GET /health` | `{"status":"ok","queue":N,"capacity":200}`, or `503` with `"degraded"` above 180 queued. Unauthenticated, so Heroku and uptime monitors can probe it. |

Anything else is a `404`. JSON, gRPC and gRPC-Web are refused: the endpoint accepts
`application/proto` only.

## Configuration

| Variable | Required | Purpose |
|----------|----------|---------|
| `MAIL_RELAY_SECRET` | yes | HMAC key shared with `chatbasket-api`. Never transmitted — see below. |
| `MAIL_RELAY_ALLOWED_IPS` | no | Comma separated addresses or CIDRs allowed to call the RPC, e.g. `203.0.113.5, 198.51.100.0/24`. Unset means any source may try. Matched against the last `X-Forwarded-For` hop, which Heroku's router appends and a caller cannot forge. |
| `SMTP_HOST` `SMTP_PORT` `SMTP_USERNAME` `SMTP_PASSWORD` `SMTP_FROM` | yes | Upstream mail server. Port `465` uses implicit TLS, anything else negotiates STARTTLS. |
| `SMTP_FROM_NAME` | no | Display name on the `From` header. |
| `PORT` | no | Set by Heroku; defaults to `8080`. |

## Request authentication

Every RPC is signed with HMAC-SHA256. The secret is only ever a key, so it never
appears on the wire, and a captured request cannot be replayed or altered.

```
X-Relay-Timestamp: <unix seconds>
X-Relay-Nonce:     <16 random bytes, hex>
X-Relay-Signature: v1=<hex hmac-sha256>
```

The signed string is these five lines joined by `\n`, with no trailing newline:

```
v1
<timestamp>
<nonce>
<request path>
<hex sha256 of the request body exactly as sent, compressed if gzipped>
```

The gateway rejects, in this order and before Connect reads a single byte of the
body: a plaintext hop (`X-Forwarded-Proto` present and not `https`), a source address
outside the allowlist, a non-`POST` method, a content type other than
`application/proto`, a body above 1 MB, and finally a missing, forged, stale (±60 s)
or already used signature. Ten rejections from one address inside a minute cut that
address off entirely for the rest of the window — with one exception: a stale
request whose signature is otherwise valid can only come from the key holder, so it
is refused (`unauthenticated: stale timestamp`, with a clock-sync hint in the log)
without counting toward that budget. A drifted clock therefore fails loudly but
never locks the backend out, and delivery resumes the moment the clock is fixed.

The signer lives in `chatbasket-api/internal/platform/clients/email.go` and the
verifier in `app/security.go`; the two `signRelayRequest` functions must stay
identical.

## Probing a deployed gateway

`buf curl` cannot be used any more — it has no way to compute the signature. This
does the same job with `curl` and `openssl`:

```bash
SECRET="$MAIL_RELAY_SECRET"
URL="$MAIL_RELAY_URL"
PROC=/rpc_core_email.v1.EmailService/SendEmail

# SendEmailRequest{to:"probe@example.com", subject:"connect probe", body:"<p>ok</p>"}
printf '\x0a\x11probe@example.com\x12\x0dconnect probe\x1a\x09<p>ok</p>' > /tmp/probe.bin

TS=$(date +%s)
NONCE=$(openssl rand -hex 16)
DIGEST=$(sha256sum /tmp/probe.bin | cut -d' ' -f1)
SIG=$(printf 'v1\n%s\n%s\n%s\n%s' "$TS" "$NONCE" "$PROC" "$DIGEST" \
  | openssl dgst -sha256 -hmac "$SECRET" -hex | awk '{print $NF}')

curl -sS -i -X POST "$URL$PROC" \
  -H "Content-Type: application/proto" \
  -H "X-Relay-Timestamp: $TS" \
  -H "X-Relay-Nonce: $NONCE" \
  -H "X-Relay-Signature: v1=$SIG" \
  --data-binary @/tmp/probe.bin
```

Expect `200` with `Content-Type: application/proto`. Sending the same request twice
returns `401 {"code":"unauthenticated","message":"replayed request"}`, which is the
replay guard doing its job.

## Development

```bash
go build ./... && go vet ./... && go test ./app/...
```

The proto contract is owned by `chatbasket-api`. To regenerate this module's stubs
after it changes, run the command recorded at the top of `buf.gen.yaml`.
