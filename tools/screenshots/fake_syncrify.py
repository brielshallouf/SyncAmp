"""Tiny stand-in for a Syncrify server: answers the anonymous build-number call so the demo
shows 'Server build 5871'. For screenshots only."""
import http.server, sys

class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        ok = self.path.startswith("/app?operation=gbn")
        self.send_response(200 if ok else 404)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()
        if ok:
            self.wfile.write(b"5871\n")
    def log_message(self, *a):
        pass

http.server.HTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
