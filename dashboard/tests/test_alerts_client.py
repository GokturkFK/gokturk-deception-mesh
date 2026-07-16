from unittest.mock import Mock, patch

import pytest
import requests

from alerts_client import fetch_alerts, format_timestamp, severity_colors, summarize


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


def test_format_timestamp_converts_zulu_to_utc_display():
    assert format_timestamp("2026-07-16T17:53:17Z") == "2026-07-16 17:53:17 UTC"


def test_severity_colors_known_and_unknown():
    assert severity_colors("Critical") == ("#ffffff", "#b91c1c")
    assert severity_colors("High") == ("#1f2937", "#f59e0b")
    assert severity_colors("weird") == ("#1f2937", "#9ca3af")


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
