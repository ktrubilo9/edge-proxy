from __future__ import annotations

import http.client
import json
import subprocess
import sys
import time
from typing import Any

HEALTH_URL = "http://127.0.0.1:9091/health"
BACKEND_A_URL = "http://backend-a:3000"
WAIT_TIMEOUT_SECONDS = 45

def compose(
        *args: str,
        capture_output: bool = False,
        check: bool = True,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["docker", "compose", *args],
        check=check,
        capture_output=capture_output,
        text=True
    )

def get_health() -> dict[str, Any]:
    result = compose(
        "exec",
        "-T",
        "reverse-proxy",
        "wget",
        "-qO-",
        HEALTH_URL,
        capture_output=True
    )
    return json.loads(result.stdout)

def wait_for_state(
        expected_active_count: int,
        expected_backend_a_active: bool,
) -> None:
    deadline = time.monotonic() + WAIT_TIMEOUT_SECONDS
    last_health: dict[str, Any] | None = None
    last_error: Exception | None = None

    while time.monotonic() < deadline:
        try:
            health = get_health()
            last_health = health

            backend_a = next(
                (
                    backend
                    for backend in health.get("backends", [])
                    if backend.get("url") == BACKEND_A_URL
                ),
                None,
            )

            print(
                "Health state:",
                json.dumps(health, separators=(",", ":")),
                flush=True
            )

            if (
                health.get("active_count") == expected_active_count
                and backend_a is not None
                and backend_a.get("active") is expected_backend_a_active
            ):
                return
        except (
            json.JSONDecodeError,
            subprocess.CalledProcessError,
        ) as error:
            last_error = error

        time.sleep(1)

    raise TimeoutError(
        "Timed out waiting for "
        f"active_count={expected_active_count},"
        f"backend-a active={expected_backend_a_active}."
        f"Last health={last_health!r}, last error={last_error!r}"
    )

def assert_proxy_responds() -> None:
    connection = http.client.HTTPConnection(
        "127.0.0.1",
        8080,
        timeout=5
    )

    try:
        connection.request(
            "GET",
            "/",
            headers={"Host": "app.example.local"},
        )

        response = connection.getresponse()
        response.read()

        if response.status != 200:
            raise AssertionError(
                f"Proxy returned HTTP {response.status}, expected 200"
            )
    finally:
        connection.close()

def main() -> None:
    print("Waiting for both backend to become healthy...")
    wait_for_state(
        expected_active_count=2,
        expected_backend_a_active=True,
    )

    assert_proxy_responds()

    try:
        print("Stopping backend-a...")
        compose("stop", "backend-a")

        print("Waiting for backend-a to become unhealthy...")
        wait_for_state(
            expected_active_count=1,
            expected_backend_a_active=False,
        )

        print('Checking routing through the remaining backend...')
        for _ in range(4):
            assert_proxy_responds()

        print("Starting backend-a...")
        compose("start", "backend-a")

        print("Waiting for backend-a to recover...")
        wait_for_state(
            expected_active_count=2,
            expected_backend_a_active=True,
        )
    finally:
        # leave the local dev stack in a usable state even if
        # assertion or timeout fails.
        compose(
            "start",
            "backend-a",
            capture_output=True,
            check=False
        )

    print("Health checker E2E test passed.")

if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"E2E test failed: {error}", file=sys.stderr)
        raise