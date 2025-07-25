#!/usr/bin/env python3
import http.server
import socketserver
import json
import sys

class MockHandler(http.server.SimpleHTTPRequestHandler):
    def do_POST(self):
        if self.path == '/api/v1/diagnostics':
            content_length = int(self.headers['Content-Length'])
            post_data = self.rfile.read(content_length)
            
            try:
                data = json.loads(post_data.decode('utf-8'))
                print(f"Received diagnostic data: {json.dumps(data, indent=2)}")
                
                # 保存接收到的数据用于验证
                with open('test/received_data.json', 'w') as f:
                    json.dump(data, f, indent=2)
                
                self.send_response(200)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({"status": "success"}).encode())
            except Exception as e:
                print(f"Error processing request: {e}")
                self.send_response(400)
                self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()
    
    def log_message(self, format, *args):
        return  # 静默日志

PORT = 8088
with socketserver.TCPServer(("", PORT), MockHandler) as httpd:
    print(f"Mock server serving at port {PORT}")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("Server stopped")
