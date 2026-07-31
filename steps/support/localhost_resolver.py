"""RFC 6761 reserves *.localhost for loopback, but not every resolver
actually implements that (e.g. plain /etc/nsswitch.conf 'dns' without
'files' or a wildcard stub). The harness's did:web hostnames
(dcs-a.localhost, dcs-b.localhost) need to resolve host-side without
editing /etc/hosts or using sudo (explicit harness constraint) — so wrap
socket.getaddrinfo: only for hostnames ending in '.localhost' that the
real resolver fails to resolve, synthesize a loopback (127.0.0.1) result
instead of raising. A no-op wherever the system resolver already handles
it (e.g. GitHub runners via systemd-resolved), since the real resolver is
always tried first.
"""

import socket


def install():
    real_getaddrinfo = socket.getaddrinfo

    def _getaddrinfo(host, port, *args, **kwargs):
        try:
            return real_getaddrinfo(host, port, *args, **kwargs)
        except socket.gaierror:
            if isinstance(host, str) and host.endswith(".localhost"):
                return real_getaddrinfo("127.0.0.1", port, *args, **kwargs)
            raise

    socket.getaddrinfo = _getaddrinfo
