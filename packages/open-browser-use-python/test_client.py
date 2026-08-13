import json
import socket
import struct
import tempfile
import threading
import unittest
from pathlib import Path

from open_browser_use.client import OpenBrowserUseClient, connect_open_browser_use, encode_frame


class OpenBrowserUseClientTest(unittest.TestCase):
    def test_encode_frame(self) -> None:
        frame = encode_frame({"id": 1, "method": "getInfo"})
        (length,) = struct.unpack("=I", frame[:4])
        self.assertEqual(length, len(frame) - 4)
        self.assertEqual(json.loads(frame[4:]), {"id": 1, "method": "getInfo"})

    def test_request_round_trip(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            socket_path = str(Path(directory) / "obu.sock")
            server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            server.bind(socket_path)
            server.listen(1)

            def serve_once() -> None:
                conn, _ = server.accept()
                with conn:
                    header = conn.recv(4)
                    (length,) = struct.unpack("=I", header)
                    payload = conn.recv(length)
                    request = json.loads(payload)
                    self.assertRegex(request["params"]["operation_id"], r"^op-python-")
                    response = {
                        "jsonrpc": "2.0",
                        "id": request["id"],
                        "result": {"name": "Open Browser Use"},
                    }
                    conn.sendall(encode_frame(response))
                server.close()

            thread = threading.Thread(target=serve_once)
            thread.start()
            client = OpenBrowserUseClient(socket_path=socket_path)
            try:
                self.assertEqual(client.get_info(), {"name": "Open Browser Use"})
            finally:
                client.close()
            thread.join(timeout=1)

    def test_file_chooser_wrapper(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            socket_path = str(Path(directory) / "obu.sock")
            server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            server.bind(socket_path)
            server.listen(1)

            def serve_once() -> None:
                conn, _ = server.accept()
                with conn:
                    header = conn.recv(4)
                    (length,) = struct.unpack("=I", header)
                    payload = conn.recv(length)
                    request = json.loads(payload)
                    self.assertEqual(request["method"], "waitForFileChooser")
                    self.assertEqual(request["params"]["tabId"], 123)
                    response = {
                        "jsonrpc": "2.0",
                        "id": request["id"],
                        "result": {"fileChooserId": "chooser-1", "isMultiple": False},
                    }
                    conn.sendall(encode_frame(response))
                server.close()

            thread = threading.Thread(target=serve_once)
            thread.start()
            client = OpenBrowserUseClient(socket_path=socket_path)
            try:
                self.assertEqual(
                    client.wait_for_file_chooser(123),
                    {"fileChooserId": "chooser-1", "isMultiple": False},
                )
            finally:
                client.close()
            thread.join(timeout=1)

    def test_download_and_clipboard_wrappers(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            socket_path = str(Path(directory) / "obu.sock")
            server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            server.bind(socket_path)
            server.listen(1)
            expected = [
                ("waitForDownload", {"tabId": 123, "timeoutMs": 5000}),
                ("downloadPath", {"downloadId": "download-1"}),
                ("readClipboardText", {"tabId": 123}),
                ("writeClipboardText", {"tabId": 123, "text": "hello"}),
            ]

            def serve() -> None:
                conn, _ = server.accept()
                with conn:
                    for method, params in expected:
                        header = conn.recv(4)
                        (length,) = struct.unpack("=I", header)
                        payload = conn.recv(length)
                        request = json.loads(payload)
                        self.assertEqual(request["method"], method)
                        for key, value in params.items():
                            self.assertEqual(request["params"][key], value)
                        response = {
                            "jsonrpc": "2.0",
                            "id": request["id"],
                            "result": {},
                        }
                        conn.sendall(encode_frame(response))
                server.close()

            thread = threading.Thread(target=serve)
            thread.start()
            client = OpenBrowserUseClient(socket_path=socket_path)
            try:
                client.wait_for_download(123, timeout_ms=5000)
                client.download_path("download-1")
                client.read_clipboard_text(123)
                client.write_clipboard_text(123, "hello")
            finally:
                client.close()
            thread.join(timeout=1)

    def test_high_level_browser_tab_helpers(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            socket_path = str(Path(directory) / "obu.sock")
            server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            server.bind(socket_path)
            server.listen(1)
            calls: list[tuple[str, str | None]] = []

            def serve() -> None:
                conn, _ = server.accept()
                with conn:
                    while True:
                        header = conn.recv(4)
                        if not header:
                            break
                        (length,) = struct.unpack("=I", header)
                        payload = conn.recv(length)
                        request = json.loads(payload)
                        method = request["method"]
                        cdp_method = request["params"].get("method")
                        calls.append((method, cdp_method))
                        result = {}
                        if method == "createTab":
                            result = {"id": 123}
                        elif method == "executeCdp" and cdp_method == "Page.navigate":
                            conn.sendall(
                                encode_frame(
                                    {
                                        "jsonrpc": "2.0",
                                        "method": "onCDPEvent",
                                        "params": {
                                            "source": {"tabId": 123},
                                            "method": "Page.domContentEventFired",
                                            "params": {},
                                        },
                                    }
                                )
                            )
                            result = {"frameId": "frame-1"}
                        elif method == "executeCdp" and cdp_method == "Runtime.evaluate":
                            expression = request["params"]["commandParams"]["expression"]
                            if "readyState" in expression:
                                result = {
                                    "result": {
                                        "value": {
                                            "href": "https://example.test/issues",
                                            "readyState": "interactive",
                                        }
                                    }
                                }
                            elif "document.title" in expression:
                                result = {"result": {"value": "Issues - open-codex-computer-use"}}
                            elif "location.href" in expression:
                                result = {"result": {"value": "https://example.test/issues"}}
                            elif "document.querySelector" in expression:
                                result = {"result": {"value": "Open\nClosed\nIssues\nStarred"}}
                            else:
                                result = {"result": {"value": "Open\nClosed\nIssues\nStarred"}}
                        conn.sendall(encode_frame({"jsonrpc": "2.0", "id": request["id"], "result": result}))
                server.close()

            thread = threading.Thread(target=serve)
            thread.start()
            browser = connect_open_browser_use(socket_path=socket_path)
            notifications = []
            browser.client.on_notification(notifications.append)
            try:
                tab = browser.new_tab()
                tab.goto("https://example.test/issues", wait_until="domcontentloaded", timeout=1)
                tab.playwright.wait_for_load_state(state="domcontentloaded", timeout=1)
                tab.playwright.wait_for_timeout(1)
                self.assertEqual(tab.title(), "Issues - open-codex-computer-use")
                self.assertEqual(tab.playwright.url(), "https://example.test/issues")
                self.assertEqual(tab.playwright.dom_snapshot(), "Open\nClosed\nIssues\nStarred")
                self.assertEqual(tab.playwright.locator("body").inner_text(timeout_ms=1000), "Open\nClosed\nIssues\nStarred")
                self.assertEqual(notifications[0]["method"], "onCDPEvent")
                self.assertEqual(
                    calls,
                    [
                        ("createTab", None),
                        ("attach", None),
                        ("executeCdp", "Page.enable"),
                        ("executeCdp", "Page.navigate"),
                        ("executeCdp", "Page.enable"),
                        ("executeCdp", "Runtime.evaluate"),
                        ("executeCdp", "Page.enable"),
                        ("executeCdp", "Runtime.evaluate"),
                        ("executeCdp", "Runtime.evaluate"),
                        ("executeCdp", "Runtime.evaluate"),
                        ("executeCdp", "Runtime.evaluate"),
                        ("executeCdp", "Runtime.evaluate"),
                    ],
                )
            finally:
                browser.close()
            thread.join(timeout=1)


if __name__ == "__main__":
    unittest.main()
