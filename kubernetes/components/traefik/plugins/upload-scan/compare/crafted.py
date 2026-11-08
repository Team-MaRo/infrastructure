# Hand-made tricky uploads (README.md): each must have every file PHP stored reach clamd byte for
# byte, and an EICAR file PHP stored must be logged infected. Exits 1 on a miss.
import base64
import http.client
import json
import os
import subprocess
import sys

PORT = int(os.environ.get("PORT", "18081"))
PREFIX = os.environ.get("PREFIX", "upload-scan-compare")
# Built from two halves, so no file holds the test string whole.
EICAR = (r"X5O!P%@AP[4\PZX54(P^)7CC)7}$" + "EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*").encode()
SKIP_MAX_BYTES = 64 << 10


def part(disposition, data, extra="", nl="\r\n"):
    return f"Content-Disposition: {disposition}{nl}{extra}{nl}".encode() + data


def body(boundary, parts, nl="\r\n", preamble=b"", close=True):
    out = preamble
    for p in parts:
        out += f"--{boundary}{nl}".encode() + p + nl.encode()
    if close:
        out += f"--{boundary}--{nl}".encode()
    return out


F = 'form-data; name="f"; filename="e.txt"'
W = "----WebKitFormBoundary7MA4YWxkTrZu0gW"
MP = "multipart/form-data; boundary=b"
CASES = [
    ("webkit boundary", f"multipart/form-data; boundary={W}", body(W, [part('form-data; name="c"', b"hi"), part(F, EICAR)])),
    ("firefox boundary", "multipart/form-data; boundary=----geckoformboundary4d1a9e", body("----geckoformboundary4d1a9e", [part(F, EICAR)])),
    ("xboundary trick", "multipart/form-data; xboundary=evil; boundary=good", body("evil", [part(F, EICAR)]) + body("good", [part('form-data; name="c"', b"hi")])),
    ("boundary parameter twice", "multipart/form-data; boundary=good; boundary=evil", body("good", [part(F, EICAR)])),
    ("quoted boundary with ;", 'multipart/form-data; boundary="a;b"', body("a;b", [part(F, EICAR)])),
    ("uppercase BOUNDARY", "multipart/form-data; BOUNDARY=up", body("up", [part(F, EICAR)])),
    ("uppercase type", "Multipart/Form-Data; boundary=b", body("b", [part(F, EICAR)])),
    ("charset before boundary", "multipart/form-data; charset=utf-8; boundary=b", body("b", [part(F, EICAR)])),
    ("no space after ;", "multipart/form-data;boundary=b", body("b", [part(F, EICAR)])),
    ("trailing space in boundary", "multipart/form-data; boundary=b ", body("b ", [part(F, EICAR)])),
    ("filename twice", MP, body("b", [part('form-data; name="f"; filename="a.php"; filename="a.php"', EICAR)])),
    ("filename*", MP, body("b", [part("form-data; name=\"f\"; filename*=UTF-8''e.txt", EICAR)])),
    ("unquoted filename with space", MP, body("b", [part("form-data; name=f; filename=a b.txt", EICAR)])),
    ("name only, no filename", MP, body("b", [part('form-data; name="f"', EICAR)])),
    ("LF line endings", MP, body("b", [part(F, EICAR, nl="\n")], nl="\n")),
    ("preamble before boundary", MP, body("b", [part(F, EICAR)], preamble=b"garbage preamble\r\n")),
    ("folded header", MP, body("b", [b'Content-Disposition: form-data; name="f";\r\n filename="e.txt"\r\n\r\n' + EICAR])),
    ("two Content-Disposition headers", MP, body("b", [b'Content-Disposition: form-data; name="c"\r\nContent-Disposition: form-data; name="f"; filename="e.txt"\r\n\r\n' + EICAR])),
    ("no closing boundary", MP, body("b", [part(F, EICAR)], close=False)),
    ("base64 transfer encoding", MP, body("b", [part(F, base64.b64encode(EICAR), extra="Content-Transfer-Encoding: base64\r\n")])),
    ("quoted-printable", MP, body("b", [part(F, EICAR, extra="Content-Transfer-Encoding: quoted-printable\r\n")])),
    ("file 22 of 25", MP, body("b", [part(f'form-data; name="f{i}"; filename="{i}.txt"', EICAR if i == 21 else b"x") for i in range(25)])),
    ("file 3 of 25", MP, body("b", [part(f'form-data; name="f{i}"; filename="{i}.txt"', EICAR if i == 2 else b"x") for i in range(25)])),
    ("array field name", MP, body("b", [part('form-data; name="f[]"; filename="a.txt"', b"x"), part('form-data; name="f[]"; filename="e.txt"', EICAR)])),
    ("leading space in type", "  multipart/form-data; boundary=b", body("b", [part(F, EICAR)])),
    ("after closing delimiter", MP, b'--b\r\nContent-Disposition: form-data; name="t"\r\n\r\nx\r\n--b--\r\n--b\r\nContent-Disposition: form-data; name="f"; filename="e.txt"\r\n\r\n' + EICAR + b"\r\n--b--\r\n"),
    ("LF delimiter in CRLF body", MP, b'--b\r\nContent-Disposition: form-data; name="t"\r\n\r\nx\n--b\nContent-Disposition: form-data; name="f"; filename="e.txt"\n\n' + EICAR + b"\n--b--\r\n"),
    ("mixed endings, LF close", MP, b'--b\r\nContent-Disposition: form-data; name="t"\r\n\r\nx\n--b\nContent-Disposition: form-data; name="f"; filename="e.txt"\n\n' + EICAR + b"\n--b--\n"),
    ("header line without colon", MP, b'--b\r\njunk\r\nContent-Disposition: form-data; name="f"; filename="e.txt"\r\n\r\n' + EICAR + b"\r\n--b--\r\n"),
    ("boundary= with a space", "multipart/form-data; boundary= abc", body(" abc", [part(F, EICAR)])),
    ("boundary = spaced", "multipart/form-data; boundary = spaced", body(" spaced", [part(F, EICAR)])),
    ("boundaryx=", "multipart/form-data; boundaryx=abc", body("abc", [part(F, EICAR)])),
    ("BOUNDARY and zboundary", "multipart/form-data; BOUNDARY=aaa; zboundary=bbb", body("bbb", [part(F, EICAR)])),
    ("delimiter after a 5120-byte line", MP, b"x" * 5120 + b"--b\r\n" + part(F, EICAR) + b"\r\n--b--\r\n"),
    ("5112-byte header line, later part", MP, body("b", [part('form-data; name="t"', b"x"), part(F, EICAR, extra="X-Pad: " + "a" * 5112 + "\r\nKey: v\r\n")])),
    ("tab before ;", "multipart/form-data\t; boundary=b", body("b", [part(F, EICAR)])),
    ("multipart/mixed, raw upload", "multipart/mixed; boundary=b", body("b", [b"Content-Type: text/plain\r\n\r\n" + EICAR])),
    ("raw body, urlencoded type, small", "application/x-www-form-urlencoded", EICAR),
    ("raw body, json type, small", "application/json", EICAR),
    ("raw body, octet-stream", "application/octet-stream", EICAR),
]


def docker_logs(name):
    out = subprocess.run(["docker", "logs", f"{PREFIX}-{name}"], capture_output=True, text=True)
    return out.stdout.splitlines() + out.stderr.splitlines()


def streams():
    return [line.split()[1] for line in docker_logs("clamd") if line.startswith("STREAM")]


def verdicts(path):
    lines = [json.loads(l) for l in docker_logs("traefik") if l.startswith("{") and "upload_scan" in l]
    return [l for l in lines if l["path"] == path]


def main():
    misses = 0
    for i, (label, content_type, data) in enumerate(CASES):
        before = len(streams())
        path = f"/crafted/{i}"
        conn = http.client.HTTPConnection("localhost", PORT, timeout=60)
        conn.request("POST", path, body=data, headers={"Host": "site.example.org", "Content-Type": content_type})
        try:
            php = json.loads(conn.getresponse().read())
        except TimeoutError:
            # A hung plugin would make every following request wait as long: stop here.
            print(f"MISS {label}: no answer within 60 s - the plugin or PHP hangs, stopping")
            sys.exit(1)
        except Exception as e:
            print(f"MISS {label}: no answer from PHP ({type(e).__name__})")
            misses += 1
            continue
        stored = [f for f in php["files"] if f["size"] > 0 and f["error"] == 0]
        got = set(streams()[before:])
        exact = all(f["sha"] in got for f in stored)
        php_eicar = any(f["eicar"] for f in php["files"]) or php["raw_eicar"]
        logged = verdicts(path)
        verdict = logged[-1]["verdict"] if logged else "(no log)"
        # Small form and JSON bodies without Content-Disposition are skipped on purpose; PHP does
        # not store them as files.
        accepted = (not stored and php["raw_eicar"] and len(data) <= SKIP_MAX_BYTES
                    and content_type.split(";")[0].strip() in ("application/x-www-form-urlencoded", "application/json"))
        # Bodies the plugin marks ambiguous are refused when blocking (phase 3), however a parser
        # reads them.
        ambiguous = logged[-1].get("ambiguous", "") if logged else ""
        ok = accepted or bool(ambiguous) or (exact and (not php_eicar or verdict == "infected"))
        misses += not ok
        why = "" if exact else "  [a stored file did not reach clamd byte for byte]"
        mark = f"  (ambiguous: {ambiguous})" if ambiguous else ""
        print(f"{'OK  ' if ok else 'MISS'} {label:34} PHP stored {len(stored)}, EICAR {php_eicar!s:5} | plugin: {verdict}{mark}{why}")
    print(f"{len(CASES)} crafted uploads, {misses} misses")
    sys.exit(1 if misses else 0)


main()
