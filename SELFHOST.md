# Self-hosting

One process, one file. The Go binary *is* the web server — the UI is compiled into
it (`go:embed`), so there is no frontend to deploy separately, no nginx to
configure, no container needed. State is a single SQLite file.

```
knowledge  ──  the binary (17 MB, no runtime deps)
knowledge.db  ──  the index (back this up; that's the whole backup story)
.env  ──  provider key + settings
```

That means "expose the WebUI" and "expose the backend" are the same job: publish
**one HTTP port**, safely.

---

## Security model — read this first

The app has **no access control of its own** beyond optional Basic auth. So:

- It binds `127.0.0.1` by default. Reaching past loopback is a deliberate choice
  (`BIND_ADDR`), and the server prints a warning if you do it without a password.
- Whoever can reach the port can read **every indexed document** and spend your
  provider key. There are no per-user permissions and no audit trail.

Pick one of the options below. They differ only in *who decides who reaches the
port*.

---

## Option 1 — Tailscale (recommended)

Best fit for "team truy cập từ mọi nơi": no open ports, no public DNS, no
certificates to manage, and the same setup whether the box is a PC in the office
or a cloud VM.

```bash
# on the machine running the binary
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up

# keep the app on loopback — Tailscale terminates TLS and proxies to it
BIND_ADDR=127.0.0.1 PORT=8080 ./bin/knowledge

# publish it inside the tailnet, with a real HTTPS certificate
sudo tailscale serve --bg 8080
sudo tailscale serve status      # prints the https://<machine>.<tailnet>.ts.net URL
```

Team members install the Tailscale app (iOS/Android/desktop), sign in with the
company account, and open that URL. Works on mobile data, hotel wifi, anywhere.

| | |
|---|---|
| Open ports | none |
| Who gets in | anyone signed into your tailnet — narrow it further with [tailnet ACLs](https://tailscale.com/kb/1018/acls) |
| TLS | automatic |
| `AUTH_PASS` | optional — the tailnet already authenticates |
| Trade-off | every user installs one app |

**Do not** use `tailscale funnel` (which puts it on the public internet) without
setting `AUTH_PASS` — Funnel bypasses tailnet identity entirely.

---

## Option 2 — Cloudflare Tunnel + Access

Best fit when the team must not install anything: they get a normal HTTPS URL and
sign in with Google/Microsoft SSO. Still no inbound ports.

```bash
cloudflared tunnel login
cloudflared tunnel create knowledge
cloudflared tunnel route dns knowledge kb.company.com

# app stays on loopback; the tunnel dials out to Cloudflare
BIND_ADDR=127.0.0.1 PORT=8080 ./bin/knowledge
cloudflared tunnel run --url http://localhost:8080 knowledge
```

Then in the Cloudflare dashboard add a **Zero Trust → Access** application for
`kb.company.com`, policy = *emails ending in @company.com*. Authentication happens
at the edge; nothing unauthenticated ever reaches the binary.

| | |
|---|---|
| Open ports | none |
| Who gets in | whoever the Access policy allows (SSO) |
| TLS | automatic |
| `AUTH_PASS` | belt-and-braces; Access is the real gate |
| Caveats | Cloudflare must not buffer the answer stream — the app already sends `Content-Type: text/event-stream`, `Cache-Control: no-cache` and `X-Accel-Buffering: no`, which is what keeps tokens flowing. Verify once with `make smoke` pointed at the public URL. |

---

## Option 3 — LAN only

Simplest, and enough if nobody needs access from outside the office.

```bash
BIND_ADDR=0.0.0.0 PORT=8080 AUTH_PASS='<a long random string>' ./bin/knowledge
# team opens http://<office-pc-ip>:8080
```

Set `AUTH_PASS` anyway: office wifi usually includes guests and phones.

> Plain HTTP on a LAN sends the Basic credential in base64 over the wire. Fine
> inside a trusted office network; not fine over the internet. For anything
> reachable from outside, put it behind option 1 or 2 so the transport is TLS.

---

## Where to run it

| Host | Good | Watch out |
|---|---|---|
| Office PC | free, data never leaves the building | must stay powered on; use Tailscale so you don't touch the office firewall |
| Company cloud VM | always up, easy to back up | the provider key and index live off-site |

Either works identically with Tailscale or Cloudflare Tunnel — that is the point
of both: no port forwarding, no static IP, no firewall change.

Sizing: idle memory is tens of MB. The heavy work is the *provider's*, not yours —
one question is one embeddings call plus one chat stream. A 2-vCPU box is ample for
a team.

---

## Run it as a service (systemd)

```ini
# /etc/systemd/system/knowledge.service
[Unit]
Description=Knowledge Engine
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=knowledge
WorkingDirectory=/opt/knowledge
EnvironmentFile=/opt/knowledge/.env
ExecStart=/opt/knowledge/bin/knowledge
Restart=always
RestartSec=3

# the process needs its own directory and nothing else
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/knowledge

[Install]
WantedBy=multi-user.target
```

```bash
sudo chmod 600 /opt/knowledge/.env      # it holds the provider key
sudo systemctl enable --now knowledge
journalctl -u knowledge -f
```

Re-indexing on a schedule (systemd timer or cron):

```bash
cd /opt/knowledge && ./bin/ingest /srv/docs   # exits non-zero if anything failed
```

---

## Operations

| Task | How |
|---|---|
| Backup | copy `knowledge.db` (stop the service, or use `sqlite3 knowledge.db ".backup out.db"`) |
| Update | build, replace the binary, `systemctl restart knowledge` — clients revalidate via ETag, so no cache flush |
| Re-index | `./bin/ingest <dir>`; re-ingesting the same path updates in place |
| Change embedding model | `EMBED_DIM` must match — delete `knowledge.db` and re-ingest, or every insert fails |
| Check a provider | `make live` (endpoints, embedding width, does chat stream) |
| Check the whole thing | `make smoke` (ingest → ask → cited answer) |
| No third-party requests | `make vendor && make build`, then `ASSET_BASE=/vendor` — the binary serves Vue/marked/DOMPurify/8bit-nes itself |

## Settings that matter for hosting

| Env | Default | Notes |
|---|---|---|
| `BIND_ADDR` | `127.0.0.1` | `0.0.0.0` to leave loopback — pair with `AUTH_PASS` |
| `PORT` | `8080` | |
| `AUTH_USER` / `AUTH_PASS` | `team` / *empty* | empty password = no auth. `/api/health` stays open either way, so probes work |
| `ASSET_BASE` | jsDelivr | `/vendor` to serve assets from the binary |
| `DB_PATH` | `knowledge.db` | |
