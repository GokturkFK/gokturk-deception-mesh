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


# --- Cok kaynakli feed (GKO-4) ---
#
# Gokturk platformunda birden fazla urun ayni Alert sozlesmesini uretir
# (hepsi gokturk-core/correlate.Alert kullanir): GOKTURK credential canary
# alarmlarini, GOKKALKAN agent alarmlarini. Roadmap'in hedefi bunlarin TEK
# panelde birlesmesi. Sekiller ozdes oldugu icin birlestirme, kaynak etiketi
# eklemekten ibaret.

# Alarma eklenen, hangi urunden geldigini gosteren anahtar. Alt cizgi ile
# baslar cunku wire contract'in parcasi DEGIL, panelin kendi ekidir.
SOURCE_SYSTEM_KEY = "_source_system"

DEFAULT_SOURCE_NAME = "GÖKTÜRK"


def parse_sources(raw: str, default_url: str = "") -> list[tuple[str, str]]:
    """`ad=url` ciftlerinden olusan virgullu listeyi cozer.

    Ornek: "GÖKTÜRK=http://control-api:8080,GÖKKALKAN=http://gokkalkan:8090"

    Ad verilmezse (duz URL) DEFAULT_SOURCE_NAME kullanilir; boylece tek
    kaynakli eski yapilandirma aynen calismaya devam eder. raw bossa
    default_url tek kaynak olarak doner.
    """
    if not raw.strip():
        return [(DEFAULT_SOURCE_NAME, default_url)] if default_url else []

    sources: list[tuple[str, str]] = []
    for chunk in raw.split(","):
        chunk = chunk.strip()
        if not chunk:
            continue
        name, sep, url = chunk.partition("=")
        if sep:
            name, url = name.strip(), url.strip()
        else:
            name, url = DEFAULT_SOURCE_NAME, chunk
        if url:
            sources.append((name, url))
    return sources


def fetch_alerts_multi(
    sources: list[tuple[str, str]], timeout: float = 5.0
) -> tuple[list[dict[str, Any]], dict[str, str]]:
    """Birden fazla kaynaktan alarmlari ceker ve tek listede birlestirir.

    Doner: (alarmlar, hatalar). Bir kaynak erisilemezse DIGERLERI yine
    gosterilir; o kaynagin hatasi `hatalar` sozlugunde adiyla doner.
    Tek bir urunun cokmesi tum SOC panelini karartmamali.

    Cikti `last_seen` alanina gore en yeni once siralanir; boylece hangi
    urunden geldigine bakilmaksizin en taze alarm ustte kalir.
    """
    merged: list[dict[str, Any]] = []
    errors: dict[str, str] = {}

    for name, url in sources:
        try:
            for alert in fetch_alerts(url, timeout=timeout):
                alert[SOURCE_SYSTEM_KEY] = name
                merged.append(alert)
        except requests.RequestException as exc:
            errors[name] = str(exc)

    merged.sort(key=lambda a: a.get("last_seen") or "", reverse=True)
    return merged, errors


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
