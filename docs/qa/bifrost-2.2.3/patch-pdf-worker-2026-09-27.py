"""Local QA workaround for server-pdf 2.0.3; keeps the bundled worker bytes."""
import base64
import hashlib
from pathlib import Path
import re
import sys

app = Path(sys.argv[1]) / "dist/mcp-app.html"
original = app.read_text()
assert hashlib.sha256(original.encode()).hexdigest() == "3c8aa8ca4d27bf20429b8b3a8dd6521340594cc1c1cbf69f50e3d35908625be7", "Unexpected upstream HTML; do not patch"
pattern = r'new URL\("data:text/javascript;base64,([A-Za-z0-9+/=]+)",import.meta.url\).href'
matches = list(re.finditer(pattern, original))
assert len(matches) == 1, "Expected exactly one bundled worker URL"
encoded = matches[0].group(1)
worker = base64.b64decode(encoded, validate=True)
assert b"WorkerMessageHandler" in worker
replacement = 'URL.createObjectURL(new Blob([Uint8Array.from(atob("' + encoded + '"),c=>c.charCodeAt(0))],{type:"text/javascript"}))'
patched = re.sub(pattern, lambda _: replacement, original)
backup = app.with_suffix(".html.before-blob-worker")
assert not backup.exists(), "Backup already exists"
backup.write_text(original)
app.write_text(patched)
print("Patched local QA PDF viewer; worker SHA256:", hashlib.sha256(worker).hexdigest())
print("HTML SHA256:", hashlib.sha256(patched.encode()).hexdigest())
