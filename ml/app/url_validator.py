"""URL validation for SSRF prevention (SEC-004-002).

Validates user-supplied URLs before fetching them in the ML sidecar
to prevent Server-Side Request Forgery attacks. All URL-fetching code
paths (PDF extraction, image OCR, voice transcription) must call
validate_fetch_url() before making HTTP requests.
"""

import ipaddress
import logging
import socket
from urllib.parse import urlparse

logger = logging.getLogger("smackerel-ml.url_validator")

_ALLOWED_SCHEMES = {"http", "https"}


class SSRFError(Exception):
    """Raised when a URL fails SSRF validation."""


def validate_fetch_url(url: str) -> str:
    """Validate that a URL is safe to fetch (SSRF prevention).

    Checks:
    1. Only http/https schemes are permitted
    2. Hostname must be present
    3. Resolved IP must not be in private/reserved ranges
    4. No userinfo (credentials) in URL

    Args:
        url: The URL to validate.

    Returns:
        The validated URL (unchanged).

    Raises:
        SSRFError: If the URL fails any validation check.
    """
    parsed = urlparse(url)

    if parsed.scheme not in _ALLOWED_SCHEMES:
        raise SSRFError(f"URL scheme must be http or https, got: {parsed.scheme!r}")

    if not parsed.hostname:
        raise SSRFError("URL must have a valid hostname")

    if parsed.username or parsed.password:
        raise SSRFError("URL must not contain credentials")

    # Resolve hostname and check against blocked IP ranges
    hostname = parsed.hostname
    _check_hostname_not_private(hostname)

    return url


def _check_hostname_not_private(hostname: str) -> None:
    """Verify that a hostname does not resolve to a private/reserved IP.

    Raises SSRFError if the hostname resolves to a blocked address.
    """
    # First check if the hostname is itself an IP literal
    try:
        addr = ipaddress.ip_address(hostname)
        if _is_blocked_ip(addr):
            raise SSRFError(f"URL resolves to blocked IP range: {addr}")
        return
    except ValueError:
        pass  # Not an IP literal, proceed with DNS resolution

    try:
        addrinfos = socket.getaddrinfo(hostname, None, socket.AF_UNSPEC, socket.SOCK_STREAM)
    except socket.gaierror as e:
        raise SSRFError(f"DNS resolution failed for {hostname}: {e}") from e

    if not addrinfos:
        raise SSRFError(f"No DNS records found for {hostname}")

    for family, _type, _proto, _canonname, sockaddr in addrinfos:
        ip_str = sockaddr[0]
        try:
            addr = ipaddress.ip_address(ip_str)
            if _is_blocked_ip(addr):
                raise SSRFError(f"URL hostname {hostname} resolves to blocked IP: {addr}")
        except ValueError:
            continue


def _is_blocked_ip(addr: ipaddress.IPv4Address | ipaddress.IPv6Address) -> bool:
    """Return True if an IP address is in a blocked range (private, loopback, link-local, metadata)."""
    if addr.is_loopback:
        return True
    if addr.is_private:
        return True
    if addr.is_reserved:
        return True
    if addr.is_link_local:
        return True
    if addr.is_multicast:
        return True

    # Block cloud metadata endpoint range explicitly (169.254.169.254)
    if isinstance(addr, ipaddress.IPv4Address):
        if addr in ipaddress.ip_network("169.254.0.0/16"):
            return True

    return False
