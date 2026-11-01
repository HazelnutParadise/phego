"""Public W-45 CLI safety and offline pipeline regressions (REQ-065, T-130)."""
import contextlib
import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

from tools import upstream_update
from tools.test_upstream_update import ROOT, _pair_archives, _snapshot, _watch_record


class UpdateBoundaryTests(unittest.TestCase):
    def invoke(self, *args):
        return subprocess.run([sys.executable, str(ROOT / "tools/upstream_update.py"), *map(str, args)],
                              cwd=ROOT, capture_output=True, text=True, timeout=4)

    def test_both_hashes_are_verified_before_either_archive_is_decoded(self):
        old, new, before, after = _pair_archives()
        after["archive_sha256"] = "0" * 64
        with mock.patch("tools.upstream_diff.read_archive", side_effect=AssertionError("decoded too early")):
            with self.assertRaisesRegex(ValueError, "sha256"):
                upstream_update.classify(old, new, before, after)

    def test_cli_rejects_fifo_symlink_and_nonobject_inputs_without_waiting(self):
        import os
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            regular = root / "lock.json"
            regular.write_text("{}")
            fifo = root / "fifo"
            os.mkfifo(fifo)
            link = root / "symlink"
            link.symlink_to(regular)
            for path in (fifo, link):
                for args in (("pair", "--lock", path, regular, regular),
                             ("pair", "--lock", regular, path, regular),
                             ("watch", path, regular)):
                    with self.subTest(args=args):
                        result = self.invoke(*args)
                        self.assertEqual(result.returncode, 2, result.stderr)
                        self.assertEqual(result.stdout, "")
                        self.assertNotIn("Traceback", result.stderr)
            for content in ("[]", "null", "[" * 1500 + "]" * 1500):
                regular.write_text(content)
                result = self.invoke("pair", "--lock", regular, regular, regular)
                self.assertEqual(result.returncode, 2)
                self.assertEqual(result.stdout, "")
                self.assertNotIn("Traceback", result.stderr)

    def test_offline_watch_binds_source_digests_and_retains_missing_source(self):
        old, new, before, after = _pair_archives()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            baseline, observed = root / "baseline.json", root / "observed.json"
            baseline.write_text(json.dumps(_snapshot({"8.5": _watch_record(before["version"], before["archive_sha256"])})))
            observed.write_text(json.dumps(_snapshot({"8.5": _watch_record(after["version"], after["archive_sha256"])})))
            (root / (hashlib.sha256(old).hexdigest() + ".tar.xz")).write_bytes(old)
            new_path = root / (hashlib.sha256(new).hexdigest() + ".tar.xz")
            new_path.write_bytes(new)
            result = self.invoke("watch", baseline, observed, "--archive-dir", root)
            self.assertEqual(result.returncode, 0, result.stderr)
            report = json.loads(result.stdout)
            self.assertEqual(report["status"], "pending_review")
            self.assertEqual(report["comparisons"][0]["report"]["new"], after)
            new_path.unlink()
            result = self.invoke("watch", baseline, observed, "--archive-dir", root)
            self.assertEqual(result.returncode, 2, result.stderr)
            blocked = json.loads(result.stdout)
            self.assertEqual(blocked["status"], "blocked")
            self.assertEqual(blocked["comparisons"][0]["status"], "blocked")
            self.assertTrue(blocked["metadata"]["changes"])
            self.assertEqual(blocked["compatibility"], "not_assessed")

    def test_cli_limits_serialized_report_before_writing_any_output(self):
        old, new, before, after = _pair_archives()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            lock, old_file, new_file = root / "lock.json", root / "old", root / "new"
            lock.write_text(json.dumps({"old": before, "new": after}))
            old_file.write_bytes(old)
            new_file.write_bytes(new)
            output, error = io.StringIO(), io.StringIO()
            with mock.patch.object(upstream_update, "classify", return_value={"oversized": "x" * (33 * 1024 * 1024)}):
                with contextlib.redirect_stdout(output), contextlib.redirect_stderr(error):
                    status = upstream_update.main(["pair", "--lock", str(lock), str(old_file), str(new_file)])
            self.assertEqual(status, 2)
            self.assertEqual(output.getvalue(), "")
            self.assertTrue(error.getvalue())

    def test_unsupported_safe_open_platform_returns_a_diagnostic(self):
        output, error = io.StringIO(), io.StringIO()
        with mock.patch.object(upstream_update.os, "O_NOFOLLOW", None, create=True):
            with contextlib.redirect_stdout(output), contextlib.redirect_stderr(error):
                status = upstream_update.main(["pair", "--lock", "unused", "old", "new"])
        self.assertEqual(status, 2)
        self.assertEqual(output.getvalue(), "")
        self.assertIn("regular-file", error.getvalue())

    def test_watch_report_budget_keeps_array_delimiters_and_blocked_diagnostic(self):
        _, _, before, after = _pair_archives()
        baseline = _snapshot({"8.5": _watch_record(before["version"], before["archive_sha256"])})
        observed = _snapshot({"8.5": _watch_record(after["version"], after["archive_sha256"])})
        with mock.patch.object(upstream_update, "classify", return_value={"padding": ""}):
            empty = upstream_update.observe_changes(baseline, observed, lambda release: b"")
        encode = lambda value: json.dumps(value, ensure_ascii=True, sort_keys=True, separators=(",", ":"))
        limit = len(encode(empty)) + 100
        with mock.patch.object(upstream_update, "MAX_REPORT", limit):
            with mock.patch.object(upstream_update, "classify", return_value={"padding": "x" * 101}):
                result = upstream_update.observe_changes(baseline, observed, lambda release: b"")
        self.assertEqual(result["status"], "blocked")
        self.assertLessEqual(len(encode(result)), limit)
        self.assertIn("size limit", result["comparisons"][0]["error"])

    def test_slow_drip_download_obeys_total_deadline(self):
        import http.client
        import socket
        import threading
        receiver, sender = socket.socketpair()
        sender.sendall(b"HTTP/1.1 200 OK\r\nContent-Length: 50\r\n\r\n")
        response = http.client.HTTPResponse(receiver)
        response.begin()

        def drip():
            try:
                for _ in range(50):
                    sender.sendall(b"x")
                    time.sleep(0.02)
            except OSError:
                pass

        worker = threading.Thread(target=drip, daemon=True)
        worker.start()
        opener = mock.Mock()
        opener.open.return_value = response
        try:
            with mock.patch("urllib.request.build_opener", return_value=opener):
                with mock.patch.object(upstream_update, "MAX_FETCH_SECONDS", 0.08):
                    start = time.monotonic()
                    with self.assertRaises((TimeoutError, OSError)):
                        upstream_update._fetch_archive({"version": "8.5.12"})
                    elapsed = time.monotonic() - start
            self.assertLess(elapsed, 0.5, "a buffered read must not reset the total deadline for every byte")
        finally:
            receiver.close()
            sender.close()
            worker.join(timeout=0.5)

    def slow_response(self, headers, fragments, begin_in_open=False):
        import http.client
        import socket
        import threading
        receiver, sender = socket.socketpair()
        receiver.settimeout(30)
        sender.sendall(headers)
        response = http.client.HTTPResponse(receiver)
        if not begin_in_open:
            response.begin()

        def drip():
            try:
                for fragment in fragments:
                    sender.sendall(fragment)
                    time.sleep(0.02)
            except OSError:
                pass

        worker = threading.Thread(target=drip, daemon=True)
        worker.start()
        opener = mock.Mock()

        def open_response(*args, **kwargs):
            if begin_in_open:
                response.begin()
            return response

        opener.open.side_effect = open_response
        try:
            with mock.patch("urllib.request.build_opener", return_value=opener):
                with mock.patch.object(upstream_update, "MAX_FETCH_SECONDS", 0.08):
                    start = time.monotonic()
                    with self.assertRaises(TimeoutError):
                        upstream_update._fetch_archive({"version": "8.5.12"})
                    self.assertLess(time.monotonic() - start, 0.5)
        finally:
            response.close()
            receiver.close()
            sender.close()
            worker.join(timeout=0.5)

    def test_chunk_size_slow_drip_obeys_total_deadline(self):
        self.slow_response(b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n",
                           [b"0"] * 50 + [b"1\r\nx\r\n0\r\n\r\n"])

    def test_valid_chunked_download_remains_supported(self):
        import http.client
        import socket
        receiver, sender = socket.socketpair()
        sender.sendall(b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n1\r\nx\r\n0\r\n\r\n")
        response = http.client.HTTPResponse(receiver)
        response.begin()
        opener = mock.Mock()
        opener.open.return_value = response
        try:
            with mock.patch("urllib.request.build_opener", return_value=opener):
                self.assertEqual(upstream_update._fetch_archive({"version": "8.5.12"}), b"x")
        finally:
            response.close()
            receiver.close()
            sender.close()

    def test_open_response_header_slow_drip_obeys_total_deadline(self):
        self.slow_response(b"HTTP/1.1 200 OK\r\nX-Slow: ",
                           [b"x"] * 50 + [b"\r\nContent-Length: 1\r\n\r\nx"], True)

    def test_protocol_errors_block_one_series_and_preserve_the_next(self):
        import http.client
        old, new, before, after = _pair_archives()
        baseline = _snapshot({"8.4": _watch_record("8.4.33", "a" * 64),
                              "8.5": _watch_record(before["version"], before["archive_sha256"])})
        observed = _snapshot({"8.4": _watch_record("8.4.34", "b" * 64),
                              "8.5": _watch_record(after["version"], after["archive_sha256"])})
        for error in (http.client.IncompleteRead(b"partial"), http.client.BadStatusLine("invalid"),
                      http.client.LineTooLong("header")):
            with self.subTest(error=type(error).__name__):
                opener = mock.Mock()
                opener.open.side_effect = error

                def load(release):
                    if release["version"].startswith("8.4"):
                        return upstream_update._fetch_archive(release)
                    return old if release["version"] == before["version"] else new

                with mock.patch("urllib.request.build_opener", return_value=opener):
                    result = upstream_update.observe_changes(baseline, observed, load)
                self.assertEqual(result["status"], "blocked")
                self.assertEqual([r["status"] for r in result["comparisons"]], ["blocked", "classified"])

    def test_online_fetch_restores_signal_state_after_success_and_error(self):
        import signal
        previous = signal.getsignal(signal.SIGALRM)
        sentinel = lambda signum, frame: None
        self.assertEqual(signal.getitimer(signal.ITIMER_REAL), (0.0, 0.0))
        signal.signal(signal.SIGALRM, sentinel)
        try:
            for error in (None, OSError("unavailable")):
                opener, response = mock.Mock(), mock.MagicMock()
                response.read1.side_effect = [b"x", b""]
                response.__enter__.return_value = response
                opener.open.return_value = response
                opener.open.side_effect = error
                with mock.patch("urllib.request.build_opener", return_value=opener):
                    if error:
                        with self.assertRaises(OSError):
                            upstream_update._fetch_archive({"version": "8.5.12"})
                    else:
                        self.assertEqual(upstream_update._fetch_archive({"version": "8.5.12"}), b"x")
                self.assertIs(signal.getsignal(signal.SIGALRM), sentinel)
                self.assertEqual(signal.getitimer(signal.ITIMER_REAL), (0.0, 0.0))
        finally:
            signal.signal(signal.SIGALRM, previous)

    def test_online_fetch_preserves_active_timer_and_blocks_nonmain_thread(self):
        import signal
        import threading
        previous = signal.getsignal(signal.SIGALRM)
        sentinel = lambda signum, frame: None
        self.assertEqual(signal.getitimer(signal.ITIMER_REAL), (0.0, 0.0))
        signal.signal(signal.SIGALRM, sentinel)
        signal.setitimer(signal.ITIMER_REAL, 60)
        try:
            with mock.patch("urllib.request.build_opener") as build:
                response = mock.MagicMock()
                response.__enter__.return_value = response
                response.read1.side_effect = [b"x", b""]
                build.return_value.open.return_value = response
                with self.assertRaises(ValueError):
                    upstream_update._fetch_archive({"version": "8.5.12"})
                build.assert_not_called()
            self.assertIs(signal.getsignal(signal.SIGALRM), sentinel)
            self.assertGreater(signal.getitimer(signal.ITIMER_REAL)[0], 50)
        finally:
            signal.setitimer(signal.ITIMER_REAL, 0)
            signal.signal(signal.SIGALRM, previous)
        errors = []

        def fetch():
            try:
                upstream_update._fetch_archive({"version": "8.5.12"})
            except ValueError as exc:
                errors.append(exc)

        with mock.patch("urllib.request.build_opener") as build:
            response = mock.MagicMock()
            response.__enter__.return_value = response
            response.read1.side_effect = [b"x", b""]
            build.return_value.open.return_value = response
            worker = threading.Thread(target=fetch)
            worker.start()
            worker.join(timeout=1)
            self.assertFalse(worker.is_alive())
            self.assertEqual(len(errors), 1)
            build.assert_not_called()
        with mock.patch.object(upstream_update.signal, "setitimer", None):
            with mock.patch("urllib.request.build_opener") as build:
                with self.assertRaisesRegex(ValueError, "POSIX"):
                    upstream_update._fetch_archive({"version": "8.5.12"})
                build.assert_not_called()


if __name__ == "__main__":
    unittest.main()
