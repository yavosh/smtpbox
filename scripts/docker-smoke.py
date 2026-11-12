"""Check the container's published HTTP, SMTP, and DNS ports."""

import json
import smtplib
import socket
import struct
import sys
import time
import urllib.request
from email.message import EmailMessage


http_port, smtp_port, dns_port = map(int, sys.argv[1:])
base_url = f"http://127.0.0.1:{http_port}"


def get_json(path):
    with urllib.request.urlopen(base_url + path, timeout=2) as response:
        return json.load(response)["result"]


def check_dns():
    query_id = 0x534D
    question = b"\x03nar\x02cy\x00" + struct.pack("!HH", 15, 1)
    query = struct.pack("!HHHHHH", query_id, 0x0100, 1, 0, 0, 0) + question
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as client:
        client.settimeout(2)
        client.connect(("127.0.0.1", dns_port))
        client.send(query)
        response = client.recv(4096)
    response_id, flags, questions, answers, _, _ = struct.unpack("!HHHHHH", response[:12])
    if response_id != query_id or flags & 0x800F != 0x8000 or questions != 1 or answers != 1:
        raise RuntimeError("DNS did not return a successful MX answer")


deadline = time.monotonic() + 30
while True:
    try:
        if get_json("/health") != {"status": "ok"}:
            raise RuntimeError("HTTP health check failed")
        with smtplib.SMTP("127.0.0.1", smtp_port, timeout=2) as client:
            if client.noop()[0] != 250:
                raise RuntimeError("SMTP readiness check failed")
        check_dns()
        break
    except (OSError, smtplib.SMTPException, ValueError, KeyError, struct.error, RuntimeError):
        if time.monotonic() >= deadline:
            raise
        time.sleep(0.5)

message = EmailMessage()
message["From"] = "sender@nar.cy"
message["To"] = "recipient@nar.cy"
message["Subject"] = "Container smoke test"
message.set_content("Captured through the published SMTP port.")
with smtplib.SMTP("127.0.0.1", smtp_port, timeout=2) as client:
    client.send_message(message)

emails = get_json("/v1/mailboxes/recipient@nar.cy/items")
if len(emails) != 1:
    raise RuntimeError("HTTP did not return the captured email")
email = emails[0]
if email["From"] != "sender@nar.cy" or email["To"] != ["recipient@nar.cy"]:
    raise RuntimeError("Captured email has unexpected sender or recipients")
if "Container smoke test" not in email["Body"] or "Captured through the published SMTP port." not in email["Body"]:
    raise RuntimeError("Captured email has an unexpected body")
print("Container smoke test passed: HTTP, SMTP capture, and DNS")
