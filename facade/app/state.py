"""Mutable per-process state for the facade."""

from __future__ import annotations

import threading
import time


class AppState:
    """Tracks runtime facts the status endpoint reports.

    Thread/async safety: counters are guarded by a lock because requests may be
    handled concurrently. The values are cheap and rarely contended.
    """

    def __init__(self, default_model: str = "") -> None:
        self._lock = threading.Lock()
        self._start = time.monotonic()
        self._requests = 0
        self._errors = 0
        self._in_flight = 0
        self._total_tokens = 0
        self._default_model = default_model

        # Rolling token-rate estimate (tokens / second) over recent completions.
        self._last_tokens = 0
        self._last_seconds = 0.0

    def uptime_seconds(self) -> float:
        return time.monotonic() - self._start

    def incr_requests(self) -> None:
        with self._lock:
            self._requests += 1

    @property
    def requests(self) -> int:
        with self._lock:
            return self._requests

    def incr_errors(self) -> None:
        with self._lock:
            self._errors += 1

    @property
    def errors(self) -> int:
        with self._lock:
            return self._errors

    def enter_request(self) -> None:
        """Mark an inference request as in-flight; pair with leave_request()."""
        with self._lock:
            self._in_flight += 1

    def leave_request(self) -> None:
        with self._lock:
            if self._in_flight > 0:
                self._in_flight -= 1

    @property
    def in_flight(self) -> int:
        with self._lock:
            return self._in_flight

    @property
    def total_tokens(self) -> int:
        """Cumulative completion tokens generated (non-streamed responses)."""
        with self._lock:
            return self._total_tokens

    @property
    def default_model(self) -> str:
        with self._lock:
            return self._default_model

    @default_model.setter
    def default_model(self, value: str) -> None:
        with self._lock:
            self._default_model = value

    def record_completion(self, tokens: int, seconds: float) -> None:
        """Record a finished generation to update the tokens/sec estimate."""
        if tokens <= 0 or seconds <= 0:
            return
        with self._lock:
            self._last_tokens = tokens
            self._last_seconds = seconds
            self._total_tokens += tokens

    @property
    def tokens_per_second(self) -> float:
        with self._lock:
            if self._last_seconds <= 0:
                return 0.0
            return self._last_tokens / self._last_seconds
