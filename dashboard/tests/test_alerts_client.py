from datetime import datetime, timezone
from unittest.mock import Mock, patch

import pytest
import requests

from alerts_client import (
    fetch_alerts,
    fetch_traps,
    format_timestamp,
    relative_time,
    severity_class,
    status_label,
    summarize,
)


def _fake_response(json_body, status=200):
    resp = Mock()
    resp.status_code = status
    resp.json.return_value = json_body
    if status >= 400:
        resp.raise_for_status.side_effect = requests.HTTPError(f"{status} error")
    else:
        resp.raise_for_status.side_effect = None
    return resp


@patch("alerts_client.requests.get")
def test_fetch_alerts_calls_expected_url(mock_get):
    mock_get.return_value = _fake_response([{"id": "a1"}])

    got = fetch_alerts("http://control-api:8080")

    mock_get.assert_called_once_with("http://control-api:8080/api/v1/alerts", timeout=5.0)
    assert got == [{"id": "a1"}]


@patch("alerts_client.requests.get")
def test_fetch_alerts_strips_trailing_slash(mock_get):
    mock_get.return_value = _fake_response([])

    fetch_alerts("http://control-api:8080/")

    mock_get.assert_called_once_with("http://control-api:8080/api/v1/alerts", timeout=5.0)


@patch("alerts_client.requests.get")
def test_fetch_alerts_raises_on_http_error(mock_get):
    mock_get.return_value = _fake_response({"error": "bad"}, status=500)

    with pytest.raises(requests.HTTPError):
        fetch_alerts("http://control-api:8080")


@patch("alerts_client.requests.get")
def test_fetch_traps_calls_expected_url(mock_get):
    mock_get.return_value = _fake_response([{"id": "t1", "username": "svc_x"}])

    got = fetch_traps("http://control-api:8080")

    mock_get.assert_called_once_with("http://control-api:8080/api/v1/traps", timeout=5.0)
    assert got == [{"id": "t1", "username": "svc_x"}]


def test_format_timestamp_converts_zulu_to_utc_display():
    assert format_timestamp("2026-07-16T17:53:17Z") == "2026-07-16 17:53:17 UTC"


def test_relative_time_buckets():
    now = datetime(2026, 7, 16, 12, 0, 0, tzinfo=timezone.utc)
    cases = {
        "2026-07-16T11:59:50Z": "az önce",
        "2026-07-16T11:57:00Z": "3 dk önce",
        "2026-07-16T09:00:00Z": "3 sa önce",
        "2026-07-11T12:00:00Z": "5 gün önce",
    }
    for raw, want in cases.items():
        assert relative_time(raw, now=now) == want


def test_relative_time_future_timestamp_clamps_to_now():
    now = datetime(2026, 7, 16, 12, 0, 0, tzinfo=timezone.utc)
    assert relative_time("2026-07-16T12:00:30Z", now=now) == "az önce"


def test_severity_class_known_and_unknown():
    assert severity_class("Critical") == "critical"
    assert severity_class("High") == "high"
    assert severity_class("weird") == "unknown"


def test_status_label_known_and_unknown():
    assert status_label("open") == "Açık"
    assert status_label("ack") == "İncelemede"
    assert status_label("closed") == "Kapalı"
    assert status_label("custom") == "custom"
    assert status_label("") == "-"


def test_summarize_counts_total_open_and_critical():
    alerts = [
        {"status": "open", "severity": "High"},
        {"status": "open", "severity": "Critical"},
        {"status": "closed", "severity": "Critical"},
    ]

    got = summarize(alerts)

    assert got == {"total": 3, "open": 2, "critical": 2}


def test_summarize_empty_list():
    assert summarize([]) == {"total": 0, "open": 0, "critical": 0}
