"""Run the existing Nushell in an isolated PTY for source-only shell tests."""
import errno
import json
import os
import pty
import re
import select
import signal
import sys
import time

spec = json.load(open(sys.argv[1], encoding="utf-8"))
pid, fd = pty.fork()
if pid == 0:
    os.chdir(spec["cwd"])
    os.environ.clear()
    os.environ.update(spec["env"])
    os.execv(spec["nu"], [spec["nu"], "-n", "--no-std-lib", "-e", spec["setup"]])
output = b""
pending = b""
index = 0
waiting = False
deadline = time.monotonic() + 15
status = None
try:
    while time.monotonic() < deadline:
        if not select.select([fd], [], [], 0.1)[0]:
            continue
        try:
            data = os.read(fd, 65536)
        except OSError as error:
            if error.errno == errno.EIO:
                break
            raise
        if not data:
            break
        output += data
        pending += data
        if index == 0 and b"Error:" in output:
            break
        if b"\x1b[6n" in data:
            os.write(fd, b"\x1b[1;1R")
        if waiting and b"\x1b]133;D" in pending:
            waiting = False
            pending = pending[pending.index(b"\x1b]133;D") :]
        if not waiting and b"READY> " in pending and index < len(spec["lines"]):
            entry = spec["lines"][index].encode()
            os.write(fd, b"\x1b[200~" + entry + b"\x1b[201~\r")
            index += 1
            waiting = True
            pending = b""
    done, status = os.waitpid(pid, os.WNOHANG)
    if not done:
        os.kill(pid, signal.SIGTERM)
        _, status = os.waitpid(pid, 0)
finally:
    os.close(fd)
text = output.decode(errors="replace").replace("\r", "\n")
text = re.sub(r"\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x1b\x07]*(\x07|\x1b\\)|\x1b[=>78]", "", text)
print(json.dumps({"status": status, "sent": index, "output": text}))
