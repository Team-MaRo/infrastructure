# A stand-in for clamd: speaks its INSTREAM command, alone or in a session (IDSESSION ... END, each
# answer with its number in front), prints the SHA-256 and size of every stream it gets, and
# answers FOUND when a stream starts with the EICAR test string (ClamAV's signature needs it at the
# start), OK otherwise.
import hashlib
import socketserver
import struct

EICAR = (r"X5O!P%@AP[4\PZX54(P^)7CC)7}$" + "EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*").encode()


def command(rfile):
    cmd = b""
    while not cmd.endswith(b"\0"):
        c = rfile.read(1)
        if not c:
            return None
        cmd += c
    return cmd


def stream(rfile):
    data = b""
    while True:
        size = struct.unpack("!L", rfile.read(4))[0]
        if size == 0:
            break
        data += rfile.read(size)
    print("STREAM", hashlib.sha256(data).hexdigest(), len(data), flush=True)
    return b"stream: Eicar-Test-Signature FOUND" if data.startswith(EICAR) else b"stream: OK"


class Stream(socketserver.StreamRequestHandler):
    def handle(self):
        cmd = command(self.rfile)
        if cmd == b"zINSTREAM\0":
            self.wfile.write(stream(self.rfile) + b"\0")
        elif cmd == b"zIDSESSION\0":
            n = 0
            while command(self.rfile) == b"zINSTREAM\0":
                n += 1
                self.wfile.write(str(n).encode() + b": " + stream(self.rfile) + b"\0")
                self.wfile.flush()


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True


Server(("", 3310), Stream).serve_forever()
