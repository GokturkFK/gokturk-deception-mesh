"""control-api'nin GET /api/v1/alerts uctan alarm cekme ve bicimlendirme mantigi.

Streamlit calisma zamanindan bagimsiz tutulur ki pytest ile test edilebilsin
(app.py sadece bu modulu cagirip render eder).
"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

import requests

# CEF/dashboard'da kullanilan severity -> (yazi rengi, arka plan rengi).
SEVERITY_COLORS: dict[str, tuple[str, str]] = {
    "Critical": ("#ffffff", "#b91c1c"),
    "High": ("#1f2937", "#f59e0b"),
}
DEFAULT_SEVERITY_COLOR = ("#1f2937", "#9ca3af")


def fetch_alerts(base_url: str, timeout: float = 5.0) -> list[dict[str, Any]]:
    """control-api'den GET /api/v1/alerts cagirir; alarm listesini doner.

    control-api bos listede de 200 + [] dondugu icin (bkz.
    cmd/control-api/alerts.go) burada None kontrolune gerek yoktur.
    """
    resp = requests.get(f"{base_url.rstrip('/')}/api/v1/alerts", timeout=timeout)
    resp.raise_for_status()
    return resp.json()


def format_timestamp(raw: str) -> str:
    """RFC3339 zaman damgasini panelde okunur bir UTC gosterime cevirir."""
    dt = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")


def severity_colors(severity: str) -> tuple[str, str]:
    """Verilen severity icin (yazi rengi, arka plan rengi) doner."""
    return SEVERITY_COLORS.get(severity, DEFAULT_SEVERITY_COLOR)


def summarize(alerts: list[dict[str, Any]]) -> dict[str, int]:
    """Ust bilgi kartlari icin toplam/acik/critical sayaclarini hesaplar."""
    return {
        "total": len(alerts),
        "open": sum(1 for a in alerts if a.get("status") == "open"),
        "critical": sum(1 for a in alerts if a.get("severity") == "Critical"),
    }
