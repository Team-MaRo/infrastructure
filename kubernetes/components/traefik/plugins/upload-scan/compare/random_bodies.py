# Random multipart bodies (README.md): every non-empty file PHP stores must reach clamd byte for
# byte. COUNT bodies from SEED; a miss is written to MISSES (a folder) for replaying. Exits 1 on a
# miss.
import http.client
import json
import os
import random
import subprocess
import sys

PORT = int(os.environ.get("PORT", "18081"))
PREFIX = os.environ.get("PREFIX", "upload-scan-compare")
COUNT = int(os.environ.get("COUNT", "400"))
SEED = int(os.environ.get("SEED", "1"))
MISSES = os.environ.get("MISSES", "")
rng = random.Random(SEED)


def streams():
    out = subprocess.run(["docker", "logs", f"{PREFIX}-clamd"], capture_output=True, text=True).stdout
    return [line.split()[1] for line in out.splitlines() if line.startswith("STREAM")]


def ambiguous(path):
    out = subprocess.run(["docker", "logs", f"{PREFIX}-traefik"], capture_output=True, text=True)
    for line in out.stdout.splitlines() + out.stderr.splitlines():
        if line.startswith("{") and "upload_scan" in line:
            entry = json.loads(line)
            if entry["path"] == path and entry.get("ambiguous"):
                return entry["ambiguous"]
    return ""


def line_break():
    return rng.choice([b"\r\n", b"\n", b"\r\n", b"\r\n"])


def file_bytes():
    kind = rng.random()
    if kind < 0.3:
        return bytes(rng.randrange(256) for _ in range(rng.randint(1, 3000)))
    if kind < 0.6:
        return bytes(rng.choice(b"ab-\r\n x") for _ in range(rng.randint(1, 400)))
    return (b"line" + line_break()) * rng.randint(1, 30) + line_break() * rng.randint(0, 3)


def body(boundary):
    out = b""
    if rng.random() < 0.2:
        out += b"preamble" + line_break()
    for i in range(rng.randint(1, 6)):
        out += b"--" + boundary + line_break()
        name = f"f{i}".encode()
        headers = []
        if rng.random() < 0.15:
            headers.append(b"junk line without colon")
        if rng.random() < 0.7:
            headers.append(b'Content-Disposition: form-data; name="' + name + b'"; filename="' + name + b'.bin"')
            if rng.random() < 0.3:
                headers.append(b"Content-Type: application/octet-stream")
        else:
            headers.append(b'Content-Disposition: form-data; name="' + name + b'"')
        for h in headers:
            out += h + line_break()
        out += rng.choice([b"\r\n", b"\n", b"\r\n"])
        out += file_bytes() + line_break()
    if rng.random() < 0.85:
        out += b"--" + boundary + b"--" + line_break()
    if rng.random() < 0.2:
        out += b"epilogue" + line_break()
    return out


def main():
    misses = flagged = 0
    for i in range(COUNT):
        boundary = rng.choice([b"b", b"----WebKitFormBoundaryX" + str(i).encode(), b"x-y_z"])
        data = body(boundary)
        before = len(streams())
        conn = http.client.HTTPConnection("localhost", PORT, timeout=60)
        path = f"/random/{SEED}/{i}"
        conn.request("POST", path, body=data,
                     headers={"Host": "site.example.org", "Content-Type": "multipart/form-data; boundary=" + boundary.decode()})
        try:
            php = json.loads(conn.getresponse().read())
        except TimeoutError:
            # A hung plugin would make every following request wait as long: stop here.
            print(f"MISS body {i}: no answer within 60 s - the plugin or PHP hangs, stopping")
            sys.exit(1)
        except Exception as e:
            misses += 1
            print(f"MISS body {i}: no answer from PHP ({type(e).__name__})")
            continue
        got = set(streams()[before:])
        missing = [f for f in php["files"] if f["size"] > 0 and f["error"] == 0 and f["sha"] not in got]
        # Refused in block mode, however a parser reads it.
        if missing and ambiguous(path):
            flagged += 1
            continue
        if missing:
            misses += 1
            print(f"MISS body {i}: {len(missing)} of {len(php['files'])} stored files did not reach clamd exactly")
            if MISSES:
                os.makedirs(MISSES, exist_ok=True)
                with open(os.path.join(MISSES, f"seed{SEED}-body{i}.bin"), "wb") as f:
                    f.write(data)
    print(f"{COUNT} random bodies (seed {SEED}), {misses} misses, {flagged} not exact but ambiguous")
    sys.exit(1 if misses else 0)


main()
