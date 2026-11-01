import io
import unittest
from unittest.mock import patch

import upstream_watch as watch


class Response(io.BytesIO):
    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()


class TestFetch(unittest.TestCase):
    def test_token_only_goes_to_github(self):
        requests = []

        class Opener:
            def open(self, request, timeout):
                requests.append(request)
                return Response(b"ok")

        with patch.dict("os.environ", {"GITHUB_TOKEN": "test-token"}), \
             patch("urllib.request.build_opener", return_value=Opener()):
            self.assertEqual(watch.fetch("https://api.github.com/repos/php/php-src/git/ref/tags/php-8.5.12"), b"ok")
            self.assertEqual(watch.fetch("https://www.php.net/releases/8_5_12.php"), b"ok")
        self.assertEqual(requests[0].get_header("Authorization"), "Bearer test-token")
        self.assertIsNone(requests[1].get_header("Authorization"))

    def test_rejects_malformed_token_without_echoing_it(self):
        token = "secret\nmalformed"
        with patch.dict("os.environ", {"GITHUB_TOKEN": token}):
            with self.assertRaises(ValueError) as error:
                watch.fetch("https://api.github.com/repos/php/php-src/git/ref/tags/php-8.5.12")
        self.assertNotIn(token, str(error.exception))
        self.assertNotIn("secret", str(error.exception))

    def test_rejects_cross_host_redirect(self):
        import urllib.request
        request = urllib.request.Request("https://api.github.com/a", headers={"Authorization": "Bearer test-token"})
        with self.assertRaises(ValueError):
            watch._SameHostRedirect().redirect_request(request, None, 302, "Found", {}, "https://www.php.net/a")


if __name__ == "__main__":
    unittest.main()
