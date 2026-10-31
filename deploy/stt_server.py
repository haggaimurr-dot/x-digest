#!/usr/bin/env python3
"""Local speech-to-text HTTP service for x-digest.

Listens on 127.0.0.1:18101 (loopback only). POST /transcribe with a
multipart form field "audio" (any ffmpeg-decodable format: webm, mp4/m4a,
wav, mp3...). Optional form field "lang" (default "zh").

Response: {"text": "..."}  (empty string when nothing recognized)

Runs under systemd as xdigest-stt.service. The model directory is
/opt/xdigest-stt/model (faster-whisper CTranslate2 format).
"""
import cgi
import json
import logging
import os
import tempfile
from http.server import BaseHTTPRequestHandler, HTTPServer

MODEL_DIR = os.environ.get("STT_MODEL_DIR", "/opt/xdigest-stt/model")
LISTEN = os.environ.get("STT_LISTEN", "127.0.0.1")
PORT = int(os.environ.get("STT_PORT", "18101"))
LANG_DEFAULT = os.environ.get("STT_LANG", "zh")

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("stt")

from faster_whisper import WhisperModel  # noqa: E402

log.info("loading model from %s ...", MODEL_DIR)
model = WhisperModel(MODEL_DIR, device="cpu", compute_type="int8")
log.info("model loaded")


class Handler(BaseHTTPRequestHandler):
    server_version = "xdigest-stt/1.0"

    def log_message(self, fmt, *args):  # quieter access log
        log.info("%s %s", self.address_string(), fmt % args)

    def _send_json(self, code, obj):
        body = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/healthz":
            self._send_json(200, {"ok": True})
        else:
            self._send_json(404, {"error": "not found"})

    def do_POST(self):
        if self.path != "/transcribe":
            self._send_json(404, {"error": "not found"})
            return
        ctype = self.headers.get("Content-Type", "")
        if "multipart/form-data" not in ctype:
            self._send_json(400, {"error": "multipart form required"})
            return
        form = cgi.FieldStorage(
            fp=self.rfile, headers=self.headers,
            environ={"REQUEST_METHOD": "POST", "CONTENT_TYPE": ctype},
        )
        if "audio" not in form:
            self._send_json(400, {"error": "field 'audio' required"})
            return
        field = form["audio"]
        lang = form.getvalue("lang") or LANG_DEFAULT
        data = field.file.read() if hasattr(field, "file") and field.file else b""
        if not data:
            self._send_json(400, {"error": "empty audio"})
            return
        if len(data) > 10 * 1024 * 1024:
            self._send_json(400, {"error": "audio too large (max 10MB)"})
            return
        suffix = os.path.splitext(field.filename or "")[1] or ".webm"
        tmp = tempfile.NamedTemporaryFile(delete=False, suffix=suffix)
        try:
            tmp.write(data)
            tmp.close()
            segments, info = model.transcribe(tmp.name, language=lang, beam_size=5)
            text = "".join(s.text for s in segments).strip()
            log.info("transcribed %.1fs audio lang=%s -> %d chars",
                     info.duration, info.language, len(text))
            self._send_json(200, {"text": text})
        except Exception as e:  # noqa: BLE001
            log.exception("transcribe failed")
            self._send_json(500, {"error": str(e)[:200]})
        finally:
            try:
                os.unlink(tmp.name)
            except OSError:
                pass


if __name__ == "__main__":
    srv = HTTPServer((LISTEN, PORT), Handler)
    log.info("listening on %s:%d", LISTEN, PORT)
    srv.serve_forever()
