"""Tests for URL validation / SSRF prevention (SEC-004-002)."""

import unittest

from app.url_validator import SSRFError, validate_fetch_url


class TestValidateFetchURL(unittest.TestCase):
    """SSRF prevention: validate_fetch_url blocks dangerous URLs."""

    def test_allows_http(self):
        # Use a public IP that won't fail DNS resolution in CI/test environments
        result = validate_fetch_url("http://93.184.216.34/file.pdf")
        self.assertEqual(result, "http://93.184.216.34/file.pdf")

    def test_allows_https(self):
        result = validate_fetch_url("https://93.184.216.34/audio.ogg")
        self.assertEqual(result, "https://93.184.216.34/audio.ogg")

    def test_blocks_file_scheme(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("file:///etc/passwd")

    def test_blocks_ftp_scheme(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("ftp://internal/data")

    def test_blocks_gopher_scheme(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("gopher://evil.host/ssrf")

    def test_blocks_empty_scheme(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("//noscheme.example.com/file")

    def test_blocks_localhost(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://localhost/admin")

    def test_blocks_127_0_0_1(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://127.0.0.1/secret")

    def test_blocks_ipv6_loopback(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://[::1]:8080/internal")

    def test_blocks_private_10_network(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://10.0.0.1/api")

    def test_blocks_private_172_network(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://172.16.0.1/internal")

    def test_blocks_private_192_168(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://192.168.1.1/admin")

    def test_blocks_cloud_metadata(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://169.254.169.254/latest/meta-data/")

    def test_blocks_url_with_credentials(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://user:pass@internal-server/data")

    def test_blocks_no_hostname(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://")

    def test_blocks_zero_ip(self):
        with self.assertRaises(SSRFError):
            validate_fetch_url("http://0.0.0.0/rss")


if __name__ == "__main__":
    unittest.main()
