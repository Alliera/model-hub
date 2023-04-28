import argparse
import json
import sys
import os
import importlib.util
import threading

import requests

from http.server import BaseHTTPRequestHandler, HTTPServer


class RequestHandler(BaseHTTPRequestHandler):
    def send_error(self, code, message=None):
        self.send_response(code)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        error_message = {
            'error': message if message else f"Error code: {code}"
        }
        self.wfile.write(json.dumps(error_message).encode('utf-8'))

    def do_POST(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)

        if self.path == '/predict':
            if not handler.model_loaded:
                self.send_error(500, 'Model not loaded')
                return

            try:
                data = json.loads(post_data.decode('utf-8'))
            except json.JSONDecodeError:
                self.send_error(400, 'Invalid request JSON data')
                return

            prediction = handler.predict(data)

            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps(prediction).encode('utf-8'))

        else:
            self.send_error(404, 'Invalid endpoint')


worker_id = ''


def log_info(text):
    print("[{}] {}".format(worker_id, text))


def log_error(text):
    print("[{}] {}".format(worker_id, text))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('worker_id', type=str)
    parser.add_argument('path', type=str)
    parser.add_argument('port', type=int)
    parser.add_argument('handler_path', type=str)
    args = parser.parse_args()
    worker_id = args.worker_id
    handler_path = os.path.abspath(args.handler_path)
    if not os.path.isfile(handler_path):
        log_error(f"Error: {args.handler_path} does not exist or is not a file")
        sys.exit(1)

    spec = importlib.util.spec_from_file_location('handler', handler_path)
    handler_module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(handler_module)

    handler = handler_module.Handler()


    def load_and_notify():
        log_info("Start loading model")
        handler.load_model(args.path)
        log_info("Model ready!")
        port = os.getenv("SERVER_PORT", default="7766")
        model_ready_url = f"http://127.0.0.1:{port}/model-ready"
        model_ready_payload = {"worker_id": args.worker_id}

        try:
            requests.post(model_ready_url, json=model_ready_payload, timeout=500)
            log_info("Notification model ready success!")
        except:
            log_info("Error: Failed to send model ready notification")


    load_thread = threading.Thread(target=load_and_notify)
    load_thread.start()

    try:
        server = HTTPServer(('127.0.0.1', args.port), RequestHandler)
        server.serve_forever()
    except KeyboardInterrupt:
        server.socket.close()
        sys.exit(0)
