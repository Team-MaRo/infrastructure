# kernel_modules

Kernel modules the nodes never load, listed in `kernel_modules_blocked`
(`defaults/main.yml`). The role writes `/etc/modprobe.d/ansible-blocked.conf` with, per
module, `install <module> /bin/false` (every load fails, also as another module's dependency)
and `blacklist <module>` (no load through an alias), and unloads a module that is loaded.

- **`sctp`** (since 2026-11-19). nginx2root (xint.io, 2026-11-18) chains code execution in an
  nginx worker with an unpatched use-after-free in the kernel's SCTP code (present since Linux
  4.15) to root on the host - also from inside a container. Any process can reach that code:
  opening an SCTP socket makes the kernel load `sctp.ko` on its own, without any capability, so
  the restricted Pod Security standard does not stop it. Our nginx instances do not meet the
  nginx bugs' conditions, but code execution in any pod would reach this path. Nothing here
  uses SCTP: every Service is TCP or UDP, and WebRTC (Matrix or Nextcloud calls, a TURN
  server) runs its SCTP inside the program over UDP, not through the kernel module.

This only stops a module from **loading**: code built into the kernel is unaffected. Debian's
cloud kernel builds SCTP as a module (`/lib/modules/<version>/kernel/net/sctp/sctp.ko.xz`),
so the line works here; check the same before adding another module.

## Running it

Tagged `kernel_modules`:

```shell
cd ansible
ansible-playbook prod.yml --tags kernel_modules
ansible prod -m shell -a 'python3 -c "import socket; socket.socket(socket.AF_INET, socket.SOCK_STREAM, 132)"'   # fails: Protocol not supported
```

To allow a module again, remove it from `kernel_modules_blocked` and run the role: the file is
rewritten without it, and the kernel loads the module on its next use.
