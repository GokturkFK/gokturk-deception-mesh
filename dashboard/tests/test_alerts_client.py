from datetime import datetime, timezone
from unittest.mock import Mock, patch

import pytest
import requests

from alerts_client import (
    DEFAULT_SOURCE_NAME,
    SOURCE_SYSTEM_KEY,
    fetch_alerts,
    fetch_alerts_multi,
    fetch_traps,
    parse_sources,
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


# --- Cok kaynakli feed (GKO-4) ---


def test_parse_sources_empty_falls_back_to_default_url():
    assert parse_sources("", "http://control-api:8080") == [
        (DEFAULT_SOURCE_NAME, "http://control-api:8080")
    ]


def test_parse_sources_empty_and_no_default_returns_nothing():
    assert parse_sources("", "") == []


def test_parse_sources_named_pairs():
    raw = "GÖKTÜRK=http://control-api:8080,GÖKKALKAN=http://gokkalkan:8090"

    assert parse_sources(raw) == [
        ("GÖKTÜRK", "http://control-api:8080"),
        ("GÖKKALKAN", "http://gokkalkan:8090"),
    ]


def test_parse_sources_bare_url_gets_default_name():
    assert parse_sources("http://control-api:8080") == [
        (DEFAULT_SOURCE_NAME, "http://control-api:8080")
    ]


def test_parse_sources_ignores_blank_chunks_and_trims():
    raw = " A=http://a , , B=http://b ,"

    assert parse_sources(raw) == [("A", "http://a"), ("B", "http://b")]


@patch("alerts_client.requests.get")
def test_fetch_alerts_multi_merges_and_tags_source(mock_get):
    def side_effect(url, timeout=5.0):
        if "control-api" in url:
            return _fake_response([{"source": "10.0.0.1", "last_seen": "2026-08-12T10:00:00Z"}])
        return _fake_response([{"source": "agent-1", "last_seen": "2026-08-12T12:00:00Z"}])

    mock_get.side_effect = side_effect

    alerts, errors = fetch_alerts_multi(
        [("GÖKTÜRK", "http://control-api:8080"), ("GÖKKALKAN", "http://gokkalkan:8090")]
    )

    assert errors == {}
    assert len(alerts) == 2
    # En yeni once siralanmali (urunden bagimsiz)
    assert alerts[0]["source"] == "agent-1"
    assert alerts[0][SOURCE_SYSTEM_KEY] == "GÖKKALKAN"
    assert alerts[1][SOURCE_SYSTEM_KEY] == "GÖKTÜRK"


@patch("alerts_client.requests.get")
def test_fetch_alerts_multi_one_source_down_others_still_shown(mock_get):
    """Bir urunun cokmesi tum SOC panelini karartmamali."""

    def side_effect(url, timeout=5.0):
        if "gokkalkan" in url:
            raise requests.ConnectionError("baglanti reddedildi")
        return _fake_response([{"source": "10.0.0.1", "last_seen": "2026-08-12T10:00:00Z"}])

    mock_get.side_effect = side_effect

    alerts, errors = fetch_alerts_multi(
        [("GÖKTÜRK", "http://control-api:8080"), ("GÖKKALKAN", "http://gokkalkan:8090")]
    )

    assert len(alerts) == 1
    assert alerts[0][SOURCE_SYSTEM_KEY] == "GÖKTÜRK"
    assert "GÖKKALKAN" in errors
    assert "baglanti reddedildi" in errors["GÖKKALKAN"]


@patch("alerts_client.requests.get")
def test_fetch_alerts_multi_all_sources_down(mock_get):
    mock_get.side_effect = requests.ConnectionError("down")

    alerts, errors = fetch_alerts_multi([("A", "http://a"), ("B", "http://b")])

    assert alerts == []
    assert set(errors) == {"A", "B"}


@patch("alerts_client.requests.get")
def test_fetch_alerts_multi_handles_missing_last_seen(mock_get):
    """last_seen olmayan alarm siralamayi patlatmamali."""
    mock_get.return_value = _fake_response([{"source": "x"}, {"source": "y", "last_seen": "2026-08-12T12:00:00Z"}])

    alerts, errors = fetch_alerts_multi([("A", "http://a")])

    assert errors == {}
    assert [a["source"] for a in alerts] == ["y", "x"]
