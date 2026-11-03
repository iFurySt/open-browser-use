#!/usr/bin/env python3
"""Bootstrap an isolated headless Zen; exercise screenshots through OBU, not WebDriver.

Requires geckodriver, an already packaged XPI, and the installed native host.
Only the disposable profile receives testing permissions. User profiles are untouched.
"""
import argparse
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--driver", required=True)
    parser.add_argument("--zen-binary", required=True)
    parser.add_argument("--xpi", required=True)
    parser.add_argument("--obu", required=True)
    parser.add_argument("--artifacts")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parent.parent
    sockets = Path("/tmp/open-browser-use")
    # The runtime checks in this script are Linux-specific; all adapter/unit tests
    # remain platform-neutral. The default native-host registry is reused.
    subprocess.run([args.obu, "version"], check=True, stdout=subprocess.DEVNULL)
    before = set(sockets.glob("*.sock"))
    with tempfile.TemporaryDirectory(prefix="obu-zen-shot-") as temporary:
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            port = reservation.getsockname()[1]
        with open(Path(temporary) / "driver.log", "w") as log:
            driver = subprocess.Popen([args.driver, "--host", "127.0.0.1", "--port", str(port), "--allow-system-access"], stdout=log, stderr=log)
            session = None

            def request(path, value=None, method=None):
                data = json.dumps(value).encode() if value is not None else None
                req = urllib.request.Request(f"http://127.0.0.1:{port}{path}", data=data, headers={"Content-Type": "application/json"}, method=method)
                try:
                    with urllib.request.urlopen(req, timeout=45) as response:
                        result = json.load(response)["value"]
                except urllib.error.HTTPError as error:
                    raise RuntimeError(error.read().decode()) from error
                if isinstance(result, dict) and "error" in result:
                    raise RuntimeError(result)
                return result

            try:
                for _ in range(100):
                    try:
                        request("/status")
                        break
                    except OSError:
                        if driver.poll() is not None:
                            raise RuntimeError("geckodriver exited before startup")
                        time.sleep(0.1)
                response = request("/session", {"capabilities": {"alwaysMatch": {
                    "browserName": "firefox",
                    "moz:firefoxOptions": {"binary": args.zen_binary, "args": ["-headless", "--no-remote"], "prefs": {
                        "browser.shell.checkDefaultBrowser": False, "browser.startup.page": 0
                    }}
                }}})
                session = response["sessionId"]
                prefix = f"/session/{session}"
                request(prefix + "/moz/addon/install", {"path": str(Path(args.xpi).resolve()), "temporary": True})
                request(prefix + "/moz/context", {"context": "chrome"})
                product = request(prefix + "/execute/sync", {"script": "return Services.appinfo.name;", "args": []})
                if product != "Zen":
                    raise RuntimeError(f"Expected Zen, got {product}")
                request(prefix + "/execute/async", {"script": """
                    const done = arguments[0];
                    const {ExtensionPermissions} = ChromeUtils.importESModule('resource://gre/modules/ExtensionPermissions.sys.mjs');
                    ExtensionPermissions.add('open-browser-use@ifuryst.com', {permissions:['userScripts'], origins:['<all_urls>']}, WebExtensionPolicy.getByID('open-browser-use@ifuryst.com').extension).then(() => done(true), error => done({error:String(error)}));
                """, "args": []})
                hostname = request(prefix + "/execute/sync", {"script": "return WebExtensionPolicy.getByID('open-browser-use@ifuryst.com').mozExtensionHostname;", "args": []})
                expected_origin = f"moz-extension://{hostname}/"
                instance = None
                for _ in range(60):
                    for path in set(sockets.glob("*.sock")) - before:
                        info = subprocess.run([args.obu, "info", "--socket", str(path), "--timeout", "2s", "--session-id", "obu-shot-bootstrap"], capture_output=True, text=True)
                        if info.returncode:
                            continue
                        result = json.loads(info.stdout).get("result", {})
                        if result.get("metadata", {}).get("extensionOrigin") == expected_origin:
                            instance = result["metadata"]["extensionInstanceId"]
                            break
                    if instance:
                        break
                    time.sleep(0.2)
                if not instance:
                    raise RuntimeError("The isolated Zen extension did not connect to its native host")
                env = dict(os.environ, OBU_LIVE_ZEN_SCREENSHOT_PROFILE=instance)
                if args.artifacts:
                    env["OBU_SCREENSHOT_ARTIFACT_DIR"] = str(Path(args.artifacts).resolve())
                print(f"Testing screenshots through OBU in isolated {product} {response['capabilities']['browserVersion']}", flush=True)
                subprocess.run(["go", "test", "./cmd/open-browser-use", "-run", "TestLiveZenScreenshot", "-count=1", "-v"], cwd=repo, env=env, check=True)
            finally:
                if session:
                    try:
                        request(f"/session/{session}", method="DELETE")
                    except (OSError, RuntimeError):
                        pass
                driver.terminate()
                try:
                    driver.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    driver.kill()
                    driver.wait()


if __name__ == "__main__":
    main()
