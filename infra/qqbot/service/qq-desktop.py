"""Expose the active QQ X display only to an SSH-forwarded VNC listener."""
import os
from pathlib import Path
import time

while True:
    for proc in Path('/proc').iterdir():
        try:
            if not proc.name.isdigit() or (proc/'comm').read_text().strip() != 'qq':
                continue
            env = dict(item.split('=', 1) for item in (proc/'environ').read_bytes().decode().split(chr(0)) if '=' in item)
            if env.get('HOME') != '/opt/qqbot-native/home' or not env.get('DISPLAY'):
                continue
            args = ['/usr/bin/x11vnc', '-display', env['DISPLAY'], '-localhost', '-rfbport', '15900', '-forever', '-shared', '-nopw', '-quiet']
            if env.get('XAUTHORITY'):
                args += ['-auth', env['XAUTHORITY']]
            os.execv(args[0], args)
        except (OSError, UnicodeError):
            continue
    time.sleep(2)

