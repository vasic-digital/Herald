#!/usr/bin/env python3
# Minimal self-signed HTTPS stub mimicking pherald /v1/* for harness self-validation.
# MODE=good → returns the responses the bank asserts on.
# MODE=bad  → returns wrong status/body so every assertion must FAIL (bite proof).
import http.server, ssl, json, os, sys, hmac, hashlib, base64, tempfile, subprocess, time

MODE = os.environ.get("STUB_MODE", "good")
SECRET = os.environ.get("HERALD_AUTH_HMAC_SECRET", "test-secret-32-bytes-of-padding!!").encode()
SEEN = set()  # idempotency by event id

def verify_jwt(auth):
    if not auth or not auth.startswith("Bearer "):
        return None
    tok = auth[len("Bearer "):]
    parts = tok.split(".")
    if len(parts) != 3:
        return None
    h, p, sig = parts
    expect = base64.urlsafe_b64encode(hmac.new(SECRET, (h+"."+p).encode(), hashlib.sha256).digest()).rstrip(b"=").decode()
    if not hmac.compare_digest(expect, sig):
        return None
    pad = "=" * (-len(p) % 4)
    try:
        claims = json.loads(base64.urlsafe_b64decode(p+pad))
    except Exception:
        return None
    return claims

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def _send(self, code, body, ctype="application/json", extra=None):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        if extra:
            for k,v in extra.items(): self.send_header(k,v)
        self.end_headers()
        if isinstance(body, (dict,list)): body = json.dumps(body)
        self.wfile.write(body.encode() if isinstance(body,str) else body)
    def do_GET(self):
        path = self.path
        if MODE == "bad":
            return self._send(500, {"status":"WRONG","flavor":"nope"})
        if path == "/v1/healthz":
            return self._send(200, {"status":"ok","flavor":"pherald","build":{"version":"0.0.0-test","commit":"abc","go_version":"go1.26"}})
        if path == "/v1/readyz":
            return self._send(200, {"status":"ready","flavor":"pherald"})
        if path == "/metrics":
            body = ('# HELP pherald_build_info Build information.\n'
                    '# TYPE pherald_build_info gauge\n'
                    'pherald_build_info{version="0.0.0-test",commit="abc",go_version="go1.26"} 1\n')
            return self._send(200, body, ctype="text/plain; version=0.0.4; charset=utf-8")
        return self._send(404, {"error":"not found"})
    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(n) if n else b""
        if MODE == "bad":
            return self._send(202, {"ok":True})  # wrong for negative cases
        if self.path != "/v1/events":
            return self._send(404, {"error":"not found"})
        claims = verify_jwt(self.headers.get("Authorization"))
        if claims is None:
            return self._send(401, {"error":"unauthorized"})
        # malformed body → event_parser tag (only after auth passes)
        try:
            ev = json.loads(raw)
        except Exception:
            return self._send(400, {"error":"event_parser: malformed JSON"})
        if "tenant" not in claims:
            return self._send(401, {"error":"runner: claims missing 'tenant'"})
        eid = ev.get("id")
        if eid in SEEN:
            return self._send(200, {"event_id":eid,"was_replay":True}, extra={"X-Herald-Replay":"true"})
        SEEN.add(eid)
        return self._send(202, {"event_id":eid,"accepted":True})

# self-signed cert
cert = tempfile.NamedTemporaryFile(suffix=".pem", delete=False).name
subprocess.run(["openssl","req","-x509","-newkey","rsa:2048","-keyout",cert,"-out",cert,
                "-days","1","-nodes","-subj","/CN=127.0.0.1"], check=True,
               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
port = int(sys.argv[1])
httpd = http.server.HTTPServer(("127.0.0.1", port), H)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(cert)
httpd.socket = ctx.wrap_socket(httpd.socket, server_side=True)
httpd.serve_forever()
