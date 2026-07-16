"""control-api'den veri cekme ve panelde gosterim icin bicimlendirme mantigi.

Streamlit calisma zamanindan bagimsiz tutulur ki pytest ile test edilebilsin
(app.py sadece bu modulu cagirip render eder).
"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

import requests

# Severity -> CSS sinif eki (bkz. app.py .gk-pill.sev-*). Bilinmeyen
# degerler notr "unknown" stiline duser.
SEVERITY_CLASSES: dict[str, str] = {
    "Critical": "critical",
    "High": "high",
}

# alerts.status -> panelde gosterilen Turkce etiket
# (degerler migrations/00001_init.sql CHECK kisitiyla ayni).
STATUS_LABELS: dict[str, str] = {
    "open": "Açık",
    "ack": "İncelemede",
    "closed": "Kapalı",
}


def fetch_alerts(base_url: str, timeout: float = 5.0) -> list[dict[str, Any]]:
    """control-api'den GET /api/v1/alerts cagirir; alarm listesini doner.

    control-api bos listede de 200 + [] dondugu icin (bkz.
    cmd/control-api/alerts.go) burada None kontrolune gerek yoktur.
    """
    resp = requests.get(f"{base_url.rstrip('/')}/api/v1/alerts", timeout=timeout)
    resp.raise_for_status()
    return resp.json()


def fetch_traps(base_url: str, timeout: float = 5.0) -> list[dict[str, Any]]:
    """control-api'den GET /api/v1/traps cagirir; tuzak listesini doner."""
    resp = requests.get(f"{base_url.rstrip('/')}/api/v1/traps", timeout=timeout)
    resp.raise_for_status()
    return resp.json()


def format_timestamp(raw: str) -> str:
    """RFC3339 zaman damgasini okunur, kesin bir UTC gosterime cevirir."""
    dt = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")


def relative_time(raw: str, now: datetime | None = None) -> str:
    """RFC3339 zamani 'az önce' / '3 dk önce' gibi goreli metne cevirir.

    now parametresi test edilebilirlik icindir; verilmezse gercek saat.
    """
    dt = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    if now is None:
        now = datetime.now(timezone.utc)
    seconds = max((now - dt).total_seconds(), 0)
    if seconds < 45:
        return "az önce"
    if seconds < 3600:
        return f"{int(seconds // 60)} dk önce"
    if seconds < 86400:
        return f"{int(seconds // 3600)} sa önce"
    return f"{int(seconds // 86400)} gün önce"


def severity_class(severity: str) -> str:
    """Verilen severity icin CSS sinif ekini doner."""
    return SEVERITY_CLASSES.get(severity, "unknown")


def status_label(status: str) -> str:
    """Alarm durumunun panelde gosterilen Turkce etiketini doner."""
    return STATUS_LABELS.get(status, status or "-")


def summarize(alerts: list[dict[str, Any]]) -> dict[str, int]:
    """Ust bilgi kartlari icin toplam/acik/critical sayaclarini hesaplar."""
    return {
        "total": len(alerts),
        "open": sum(1 for a in alerts if a.get("status") == "open"),
        "critical": sum(1 for a in alerts if a.get("severity") == "Critical"),
    }
