# A stand-in for clamd: speaks its INSTREAM command, prints the SHA-256 and size of every stream it
# gets, and answers FOUND when a stream starts with the EICAR test string (ClamAV's signature
# needs it at the start), OK otherwise.
import hashlib
import socketserver
import struct

EICAR = (r"X5O!P%@AP[4\PZX54(P^)7CC)7}$" + "EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*").encode()


class Stream(socketserver.StreamRequestHandler):
    def handle(self):
        if self.rfile.read(10) != b"zINSTREAM\0":
            return
        data = b""
        while True:
            size = struct.unpack("!L", self.rfile.read(4))[0]
            if size == 0:
                break
            data += self.rfile.read(size)
        print("STREAM", hashlib.sha256(data).hexdigest(), len(data), flush=True)
        found = data.startswith(EICAR)
        self.wfile.write(b"stream: Eicar-Test-Signature FOUND\0" if found else b"stream: OK\0")


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True


Server(("", 3310), Stream).serve_forever()
