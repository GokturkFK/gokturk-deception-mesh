"""GÖKTÜRK Deception Mesh — canlı alarm paneli (APP-9).

control-api'nin GET /api/v1/alerts'ini periyodik olarak çeker ve
severity/kaynak/ATT&CK tekniği/ilk-son görülme bilgisiyle gösterir.
AC (PROJECT PLAN.md APP-9): alarm oluşunca panelde ≤5 sn içinde belirir.

Tasarım notu: panel air-gapped ortamda da çalışmalı (PLAN OPS-7), bu
yüzden harici font/CDN yok — sistem font stack'i + inline CSS kullanılır.
"""

import html
import os
import re
import time
from datetime import datetime

import requests
import streamlit as st

from alerts_client import (
    SOURCE_SYSTEM_KEY,
    fetch_alerts_multi,
    fetch_traps,
    format_timestamp,
    parse_sources,
    relative_time,
    severity_class,
    status_label,
    summarize,
)

CONTROL_API_URL = os.environ.get("CONTROL_API_URL", "http://localhost:8080")
REFRESH_SECONDS = int(os.environ.get("REFRESH_SECONDS", "3"))

# GKO-4: panel birden fazla urunun alarmini ayni feed'de gosterebilir.
# Bos birakilirsa yalnizca CONTROL_API_URL kullanilir — mevcut tek urunlu
# kurulumlar hicbir degisiklik gerektirmeden aynen calisir.
#   ALERT_SOURCES="GÖKTÜRK=http://control-api:8080,GÖKKALKAN=http://gokkalkan:8090"
ALERT_SOURCES = parse_sources(os.environ.get("ALERT_SOURCES", ""), CONTROL_API_URL)
MULTI_SOURCE = len(ALERT_SOURCES) > 1

TECHNIQUE_RE = re.compile(r"^T\d{4}(\.\d{3})?$")
# MITRE ATLAS (AI/agent) teknik kodlari: AML.T0053, AML.T0110.000 ...
# GOKKALKAN bunlari uretiyor (bkz. gokkalkan/docs/DECISIONS.md Karar 1).
ATLAS_TECHNIQUE_RE = re.compile(r"^AML\.T\d{4}(\.\d{3})?$")

st.set_page_config(page_title="GÖKTÜRK — Deception Mesh", page_icon="🦅", layout="wide")

st.markdown(
    """
    <style>
    /* Streamlit kromunu gizle — panel kiosk/SOC ekraninda tek basina durur */
    #MainMenu, footer, [data-testid="stHeader"] { display: none; }
    .block-container, [data-testid="stMainBlockContainer"] {
        padding-top: 2.2rem; max-width: 1180px;
    }

    :root {
        --bg: #0A111F;
        --surface: #121C30;
        --surface2: #0E1626;
        --line: rgba(148, 170, 200, .16);
        --text: #E8EFF9;
        --muted: #93A5BE;
        --accent: #45C4D3;          /* Göktürk turkuazı */
        --crit: #FF6067;
        --crit-bg: rgba(239, 68, 68, .13);
        --high: #F5B14C;
        --high-bg: rgba(245, 177, 76, .12);
        --live: #3DDC97;
        --mono: ui-monospace, "Cascadia Mono", Consolas, "Courier New", monospace;
    }

    /* Başlık */
    .gk-header {
        display: flex; align-items: flex-end; justify-content: space-between;
        gap: 1rem; flex-wrap: wrap;
        border-bottom: 1px solid var(--line);
        padding-bottom: 1.1rem; margin-bottom: 1.4rem;
    }
    .gk-eyebrow {
        font-size: .72rem; letter-spacing: .24em; text-transform: uppercase;
        color: var(--accent); font-weight: 600; margin-bottom: .35rem;
    }
    .gk-title {
        font-size: 1.7rem; font-weight: 700; color: var(--text);
        letter-spacing: .01em; line-height: 1.1;
    }
    .gk-sub { color: var(--muted); font-size: .86rem; margin-top: .45rem; }
    .gk-live { display: flex; flex-direction: column; align-items: flex-end; gap: .4rem; }
    .gk-live-pill {
        display: inline-flex; align-items: center; gap: .45rem;
        border: 1px solid rgba(61, 220, 151, .35); background: rgba(61, 220, 151, .08);
        color: var(--live); border-radius: 999px; padding: .28rem .8rem;
        font-size: .72rem; font-weight: 700; letter-spacing: .14em;
    }
    .gk-live-pill.err {
        border-color: rgba(239, 68, 68, .4);
        background: var(--crit-bg); color: var(--crit);
    }
    .gk-live-dot {
        width: 8px; height: 8px; border-radius: 50%; background: var(--live);
        box-shadow: 0 0 0 0 rgba(61, 220, 151, .5);
        animation: gk-pulse 2s infinite;
    }
    .gk-live-dot.err { background: var(--crit); animation: none; }
    @keyframes gk-pulse {
        70%  { box-shadow: 0 0 0 7px rgba(61, 220, 151, 0); }
        100% { box-shadow: 0 0 0 0 rgba(61, 220, 151, 0); }
    }
    @media (prefers-reduced-motion: reduce) { .gk-live-dot { animation: none; } }
    .gk-live-meta { color: var(--muted); font-size: .72rem; font-family: var(--mono); }

    /* İstatistik kartları */
    .gk-cards {
        display: grid; grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
        gap: .9rem; margin-bottom: 1.5rem;
    }
    .gk-card {
        background: var(--surface); border: 1px solid var(--line);
        border-radius: 10px; padding: 1rem 1.15rem;
    }
    .gk-card-label {
        font-size: .68rem; letter-spacing: .16em; text-transform: uppercase;
        color: var(--muted); margin-bottom: .5rem; font-weight: 600;
    }
    .gk-card-value {
        font-size: 1.9rem; font-weight: 700; color: var(--text);
        font-variant-numeric: tabular-nums; line-height: 1;
    }
    .gk-card-value.crit { color: var(--crit); }
    .gk-card-value.accent { color: var(--accent); }

    /* Alarm tablosu */
    .gk-tablewrap {
        background: var(--surface); border: 1px solid var(--line);
        border-radius: 12px; overflow-x: auto;
    }
    table.gk-alerts { width: 100%; border-collapse: collapse; font-size: .87rem; }
    .gk-alerts th {
        text-align: left; font-size: .66rem; letter-spacing: .14em;
        text-transform: uppercase; color: var(--muted); font-weight: 600;
        padding: .75rem 1rem; border-bottom: 1px solid var(--line); white-space: nowrap;
    }
    .gk-alerts td {
        padding: .7rem 1rem; border-bottom: 1px solid rgba(148, 170, 200, .08);
        color: var(--text); white-space: nowrap; vertical-align: middle;
    }
    .gk-alerts tbody tr:last-child td { border-bottom: none; }
    .gk-alerts tbody tr:hover { background: rgba(69, 196, 211, .045); }
    tr.sev-critical td:first-child { box-shadow: inset 3px 0 0 var(--crit); }
    tr.sev-high td:first-child { box-shadow: inset 3px 0 0 var(--high); }

    .gk-pill {
        display: inline-block; padding: .18rem .65rem; border-radius: 999px;
        font-size: .7rem; font-weight: 700; letter-spacing: .08em;
    }
    .gk-pill.sev-critical { color: var(--crit); background: var(--crit-bg); border: 1px solid rgba(239, 68, 68, .35); }
    .gk-pill.sev-high { color: var(--high); background: var(--high-bg); border: 1px solid rgba(245, 177, 76, .35); }
    .gk-pill.sev-unknown { color: var(--muted); background: rgba(148, 170, 200, .1); border: 1px solid var(--line); }

    .gk-ip { font-family: var(--mono); font-size: .85rem; }
    /* !important: Streamlit'in kendi <a> stili (renk+alt cizgi) siniftan agir basiyor */
    a.gk-chip, a.gk-chip:visited {
        font-family: var(--mono); font-size: .76rem;
        color: var(--accent) !important;
        border: 1px solid rgba(69, 196, 211, .35); background: rgba(69, 196, 211, .08);
        padding: .14rem .5rem; border-radius: 6px;
        text-decoration: none !important;
    }
    a.gk-chip:hover { background: rgba(69, 196, 211, .16); }
    /* ATLAS rozeti: tiklanabilir DEGIL (atlas.mitre.org derin yol servis
       etmiyor), bu yuzden notr renkte ve imlecsiz. */
    .gk-chip-atlas {
        display: inline-block; font-family: var(--mono); font-size: .76rem;
        color: var(--muted); border: 1px solid var(--line);
        background: rgba(148, 170, 200, .08);
        padding: .14rem .5rem; border-radius: 6px;
    }
    .gk-status { font-size: .8rem; }
    .gk-status.st-open { color: #FFB4B8; }
    .gk-status.st-ack { color: var(--accent); }
    .gk-status.st-closed { color: var(--muted); }
    .gk-count { font-family: var(--mono); font-variant-numeric: tabular-nums; }
    /* Kaynak sistem sutunu (GKO-4): yalnizca cok urunlu kurulumda cikar */
    .gk-sys { font-size: .78rem; color: var(--accent); letter-spacing: .04em; white-space: nowrap; }
    .gk-time { color: var(--muted); font-size: .8rem; font-variant-numeric: tabular-nums; }

    /* Boş / hata durumları, dipnot */
    .gk-empty {
        border: 1px dashed var(--line); border-radius: 12px;
        padding: 2.8rem 1rem; text-align: center;
        color: var(--muted); background: var(--surface2);
    }
    .gk-empty-title { color: var(--text); font-weight: 600; margin-bottom: .35rem; }
    .gk-banner-err {
        border: 1px solid rgba(239, 68, 68, .4); background: var(--crit-bg);
        color: #FFACAF; padding: .8rem 1rem; border-radius: 10px;
        font-size: .86rem; margin-bottom: 1rem;
    }
    .gk-foot { color: var(--muted); font-size: .72rem; margin-top: 1.2rem; }
    </style>
    """,
    unsafe_allow_html=True,
)

# Bir kaynak dusse bile digerleri gosterilir; hatalar ayrica bildirilir.
alerts, source_errors = fetch_alerts_multi(ALERT_SOURCES)
err = "; ".join(f"{name}: {msg}" for name, msg in source_errors.items()) or None

try:
    traps = fetch_traps(CONTROL_API_URL)
    active_traps = str(sum(1 for t in traps if not t.get("revoked_at")))
except requests.RequestException:
    active_traps = "–"

# Tum kaynaklar dustuyse "baglanti yok"; bir kismi ayaktaysa panel canli
# sayilir ama hata bandi yine gosterilir.
all_down = bool(ALERT_SOURCES) and len(source_errors) == len(ALERT_SOURCES)
if all_down:
    live_pill = (
        '<span class="gk-live-pill err"><span class="gk-live-dot err"></span>BAĞLANTI YOK</span>'
    )
else:
    live_pill = '<span class="gk-live-pill"><span class="gk-live-dot"></span>CANLI</span>'

source_meta = (
    " · ".join(name for name, _ in ALERT_SOURCES) if MULTI_SOURCE else CONTROL_API_URL
)

st.markdown(
    f"""
    <div class="gk-header">
      <div>
        <div class="gk-eyebrow">Deception Mesh</div>
        <div class="gk-title">GÖKTÜRK</div>
        <div class="gk-sub">Credential-canary alarm akışı · MITRE ATT&amp;CK korelasyonu</div>
      </div>
      <div class="gk-live">
        {live_pill}
        <span class="gk-live-meta">{html.escape(source_meta)} · {datetime.now().strftime("%H:%M:%S")}</span>
      </div>
    </div>
    """,
    unsafe_allow_html=True,
)

if err:
    st.markdown(
        f'<div class="gk-banner-err">Alarm kaynağına ulaşılamadı — {html.escape(err)}</div>',
        unsafe_allow_html=True,
    )

counts = summarize(alerts)
crit_cls = " crit" if counts["critical"] else ""
open_cls = " crit" if counts["open"] else ""
st.markdown(
    f"""
    <div class="gk-cards">
      <div class="gk-card">
        <div class="gk-card-label">Toplam Alarm</div>
        <div class="gk-card-value">{counts["total"]}</div>
      </div>
      <div class="gk-card">
        <div class="gk-card-label">Açık Alarm</div>
        <div class="gk-card-value{open_cls}">{counts["open"]}</div>
      </div>
      <div class="gk-card">
        <div class="gk-card-label">Critical</div>
        <div class="gk-card-value{crit_cls}">{counts["critical"]}</div>
      </div>
      <div class="gk-card">
        <div class="gk-card-label">Aktif Tuzak</div>
        <div class="gk-card-value accent">{html.escape(active_traps)}</div>
      </div>
    </div>
    """,
    unsafe_allow_html=True,
)


def render_row(a: dict) -> str:
    sev = a.get("severity", "")
    sev_cls = severity_class(sev)

    tech = a.get("technique") or ""
    if TECHNIQUE_RE.match(tech):
        tech_url = f"https://attack.mitre.org/techniques/{tech.replace('.', '/')}/"
        tech_cell = (
            f'<a class="gk-chip" href="{tech_url}" target="_blank" '
            f'rel="noopener">{html.escape(tech)}</a>'
        )
    elif ATLAS_TECHNIQUE_RE.match(tech):
        # GÖKKALKAN gibi agent/AI ürünleri MITRE ATLAS kodları üretir
        # (AML.T....). Bunlar için LINK VERİLMEZ: atlas.mitre.org istemci
        # tarafında render edilen bir SPA ve derin yolları sunucudan
        # servis etmiyor — /techniques/AML.T0053 dahil hepsi 404 dönüyor
        # (ölçüldü). Kırık link vermektense sadece rozet gösteriliyor.
        tech_cell = f'<span class="gk-chip gk-chip-atlas">{html.escape(tech)}</span>'
    else:
        tech_cell = html.escape(tech) if tech else "—"

    status = a.get("status", "")
    first_seen, last_seen = a["first_seen"], a["last_seen"]
    # Kaynak sistem sutunu yalnizca birden fazla urun bagliyken anlamli;
    # tek urunlu kurulumda panel eskisiyle birebir ayni gorunur.
    system_cell = (
        f'<td class="gk-sys">{html.escape(a.get(SOURCE_SYSTEM_KEY, "—"))}</td>'
        if MULTI_SOURCE
        else ""
    )
    return (
        f'<tr class="sev-{sev_cls}">'
        f'<td><span class="gk-pill sev-{sev_cls}">{html.escape(sev.upper() or "?")}</span></td>'
        f"{system_cell}"
        f'<td class="gk-ip">{html.escape(a.get("source", ""))}</td>'
        f"<td>{tech_cell}</td>"
        f'<td><span class="gk-status st-{html.escape(status)}">{html.escape(status_label(status))}</span></td>'
        f'<td class="gk-count">{int(a.get("trip_count", 0))}×</td>'
        f'<td class="gk-time" title="{html.escape(format_timestamp(first_seen))}">{html.escape(relative_time(first_seen))}</td>'
        f'<td class="gk-time" title="{html.escape(format_timestamp(last_seen))}">{html.escape(relative_time(last_seen))}</td>'
        f"</tr>"
    )


if not alerts:
    if not err:
        st.markdown(
            f"""
            <div class="gk-empty">
              <div class="gk-empty-title">Şu an alarm yok — mesh sessiz</div>
              <div>Bir canary tetiklendiğinde alarm bu ekranda {REFRESH_SECONDS} sn içinde belirir.</div>
            </div>
            """,
            unsafe_allow_html=True,
        )
else:
    rows = "".join(render_row(a) for a in alerts)
    st.markdown(
        f"""
        <div class="gk-tablewrap">
        <table class="gk-alerts">
          <thead><tr>
            <th>Önem</th>{"<th>Sistem</th>" if MULTI_SOURCE else ""}<th>Kaynak</th><th>Teknik</th><th>Durum</th>
            <th>Trip</th><th>İlk görülme</th><th>Son görülme</th>
          </tr></thead>
          <tbody>{rows}</tbody>
        </table>
        </div>
        """,
        unsafe_allow_html=True,
    )

st.markdown(
    f'<div class="gk-foot">GÖKTÜRK Deception Mesh · deterministik korelasyon, sıfır-FP tezi · '
    f"veriler {REFRESH_SECONDS} sn'de bir yenilenir</div>",
    unsafe_allow_html=True,
)

time.sleep(REFRESH_SECONDS)
st.rerun()
