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
        padding-top: 2.4rem; max-width: 1200px;
    }

    /*
     * Kasitli olarak ozel bir webfont YOK. Panel her REFRESH_SECONDS'ta
     * (varsayilan 3sn) tam sayfa yeniden render ediliyor (bkz. dosya sonu
     * st.rerun()); base64 gomulu bir font o dongude tekrar tekrar
     * gonderilirdi (olçüldü: Cascadia Mono TTF ~360KB, her 3 saniyede
     * bir). Bunun yerine sistem font stack'i + tip olcegi/hiyerarsi/renkle
     * kurumsal gorunum kuruluyor — hem air-gapped (OPS-7) kisitina hem de
     * bu render dongusune uygun.
     */
    :root {
        --bg: #080D18;
        --bg-deep: #050912;
        --surface: #121C30;
        --surface-raised: #16223A;
        --surface2: #0E1626;
        --line: rgba(148, 170, 200, .14);
        --line-strong: rgba(148, 170, 200, .26);
        --text: #EDF3FB;
        --muted: #8CA0BC;
        --faint: #5C7092;
        --accent: #45C4D3;          /* Göktürk turkuazı */
        --accent-dim: rgba(69, 196, 211, .12);
        --crit: #FF6B72;
        --crit-bg: rgba(255, 90, 97, .12);
        --high: #F5B14C;
        --high-bg: rgba(245, 177, 76, .12);
        --live: #3DDC97;
        --radius-lg: 14px;
        --radius-md: 10px;
        --radius-sm: 6px;
        --mono: ui-monospace, "Cascadia Mono", Consolas, "Courier New", monospace;
        --sans: -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    }

    /* Komuta-merkezi zemini: koyu dikey gradyan + cok soluk radar izgarasi.
       Tumu CSS ile uretiliyor (goruntu/font dosyasi yok — air-gapped). */
    [data-testid="stAppViewContainer"], .stApp {
        background:
            radial-gradient(1100px 480px at 82% -12%, rgba(69, 196, 211, .07), transparent 60%),
            linear-gradient(180deg, var(--bg) 0%, var(--bg-deep) 100%);
        background-attachment: fixed;
    }
    [data-testid="stAppViewContainer"]::before {
        content: ""; position: fixed; inset: 0; pointer-events: none; z-index: 0;
        background-image:
            linear-gradient(rgba(148, 170, 200, .05) 1px, transparent 1px),
            linear-gradient(90deg, rgba(148, 170, 200, .05) 1px, transparent 1px);
        background-size: 42px 42px;
        -webkit-mask-image: radial-gradient(ellipse 90% 60% at 50% 0%, black 0%, transparent 70%);
        mask-image: radial-gradient(ellipse 90% 60% at 50% 0%, black 0%, transparent 70%);
    }
    body, .stApp, .stMarkdown, p, span, div { font-family: var(--sans); }

    /* Başlık */
    .gk-header {
        position: relative; z-index: 1;
        display: flex; align-items: flex-end; justify-content: space-between;
        gap: 1rem; flex-wrap: wrap;
        border-bottom: 1px solid var(--line);
        padding-bottom: 1.2rem; margin-bottom: 1.6rem;
    }
    .gk-brand { display: flex; align-items: center; gap: .85rem; }
    .gk-mark {
        width: 40px; height: 40px; flex-shrink: 0;
        filter: drop-shadow(0 0 10px rgba(69, 196, 211, .35));
    }
    .gk-eyebrow {
        font-size: .7rem; letter-spacing: .26em; text-transform: uppercase;
        color: var(--accent); font-weight: 600; margin-bottom: .3rem;
    }
    .gk-title {
        font-size: 1.85rem; font-weight: 800; color: var(--text);
        letter-spacing: .01em; line-height: 1.1;
    }
    .gk-sub { color: var(--muted); font-size: .87rem; margin-top: .5rem; }
    .gk-live { display: flex; flex-direction: column; align-items: flex-end; gap: .5rem; }
    .gk-live-pill {
        display: inline-flex; align-items: center; gap: .5rem;
        border: 1px solid rgba(61, 220, 151, .35); background: rgba(61, 220, 151, .08);
        color: var(--live); border-radius: 999px; padding: .32rem .9rem .32rem .7rem;
        font-size: .72rem; font-weight: 700; letter-spacing: .14em;
    }
    .gk-live-pill.err {
        border-color: rgba(255, 90, 97, .4);
        background: var(--crit-bg); color: var(--crit);
    }
    .gk-live-dot {
        width: 7px; height: 7px; border-radius: 50%; background: var(--live);
        box-shadow: 0 0 0 0 rgba(61, 220, 151, .5);
        animation: gk-pulse 2s infinite;
    }
    .gk-live-dot.err { background: var(--crit); animation: none; }
    @keyframes gk-pulse {
        70%  { box-shadow: 0 0 0 7px rgba(61, 220, 151, 0); }
        100% { box-shadow: 0 0 0 0 rgba(61, 220, 151, 0); }
    }
    @media (prefers-reduced-motion: reduce) { .gk-live-dot { animation: none; } }
    .gk-live-meta { color: var(--faint); font-size: .72rem; font-family: var(--mono); }

    /* İstatistik kartları */
    .gk-cards {
        position: relative; z-index: 1;
        display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
        gap: 1rem; margin-bottom: 1.6rem;
    }
    .gk-card {
        position: relative; overflow: hidden;
        background: linear-gradient(165deg, var(--surface-raised), var(--surface));
        border: 1px solid var(--line);
        border-radius: var(--radius-lg); padding: 1.1rem 1.25rem;
        transition: border-color .15s ease, transform .15s ease;
    }
    .gk-card::before {
        content: ""; position: absolute; inset: 0 0 auto 0; height: 2px;
        background: var(--card-accent, var(--line-strong));
        opacity: .8;
    }
    .gk-card:hover { border-color: var(--line-strong); transform: translateY(-1px); }
    .gk-card.is-crit { --card-accent: var(--crit); }
    .gk-card.is-accent { --card-accent: var(--accent); }
    .gk-card-top {
        display: flex; align-items: center; justify-content: space-between;
        margin-bottom: .8rem;
    }
    .gk-card-label {
        font-size: .67rem; letter-spacing: .16em; text-transform: uppercase;
        color: var(--muted); font-weight: 700;
    }
    .gk-card-icon { color: var(--faint); opacity: .8; }
    .gk-card.is-crit .gk-card-icon { color: var(--crit); opacity: 1; }
    .gk-card.is-accent .gk-card-icon { color: var(--accent); opacity: 1; }
    .gk-card-value {
        font-family: var(--mono);
        font-size: 2rem; font-weight: 700; color: var(--text);
        font-variant-numeric: tabular-nums; line-height: 1;
    }
    .gk-card-value.crit { color: var(--crit); }
    .gk-card-value.accent { color: var(--accent); }

    /* Alarm tablosu */
    .gk-tablewrap {
        position: relative; z-index: 1;
        background: var(--surface); border: 1px solid var(--line);
        border-radius: var(--radius-lg); overflow: hidden;
        box-shadow: 0 20px 40px -24px rgba(0, 0, 0, .6);
    }
    .gk-table-scroll { overflow-x: auto; }
    .gk-table-head {
        display: flex; align-items: center; justify-content: space-between;
        padding: .9rem 1.2rem; border-bottom: 1px solid var(--line);
        background: var(--surface-raised);
    }
    .gk-table-head-label {
        font-size: .68rem; letter-spacing: .18em; text-transform: uppercase;
        color: var(--muted); font-weight: 700;
    }
    table.gk-alerts { width: 100%; border-collapse: collapse; font-size: .87rem; }
    .gk-alerts th {
        text-align: left; font-size: .66rem; letter-spacing: .14em;
        text-transform: uppercase; color: var(--faint); font-weight: 700;
        padding: .7rem 1.2rem; border-bottom: 1px solid var(--line); white-space: nowrap;
    }
    .gk-alerts td {
        padding: .75rem 1.2rem; border-bottom: 1px solid rgba(148, 170, 200, .07);
        color: var(--text); white-space: nowrap; vertical-align: middle;
    }
    .gk-alerts tbody tr:last-child td { border-bottom: none; }
    .gk-alerts tbody tr { transition: background-color .12s ease; }
    .gk-alerts tbody tr:hover { background: rgba(69, 196, 211, .05); }
    tr.sev-critical td:first-child { box-shadow: inset 3px 0 0 var(--crit); }
    tr.sev-high td:first-child { box-shadow: inset 3px 0 0 var(--high); }

    .gk-pill {
        display: inline-flex; align-items: center; gap: .4rem;
        padding: .22rem .7rem .22rem .55rem; border-radius: 999px;
        font-size: .68rem; font-weight: 700; letter-spacing: .07em;
    }
    .gk-pill::before {
        content: ""; width: 6px; height: 6px; border-radius: 50%;
        background: currentColor; flex-shrink: 0;
    }
    .gk-pill.sev-critical { color: var(--crit); background: var(--crit-bg); border: 1px solid rgba(255, 90, 97, .32); }
    .gk-pill.sev-high { color: var(--high); background: var(--high-bg); border: 1px solid rgba(245, 177, 76, .32); }
    .gk-pill.sev-unknown { color: var(--muted); background: rgba(148, 170, 200, .08); border: 1px solid var(--line); }

    .gk-ip { font-family: var(--mono); font-size: .85rem; color: var(--text); }
    /* !important: Streamlit'in kendi <a> stili (renk+alt cizgi) siniftan agir basiyor */
    a.gk-chip, a.gk-chip:visited {
        font-family: var(--mono); font-size: .76rem;
        color: var(--accent) !important;
        border: 1px solid rgba(69, 196, 211, .32); background: var(--accent-dim);
        padding: .16rem .55rem; border-radius: var(--radius-sm);
        text-decoration: none !important; transition: background-color .12s ease;
    }
    a.gk-chip:hover { background: rgba(69, 196, 211, .18); }
    /* ATLAS rozeti: tiklanabilir DEGIL (atlas.mitre.org derin yol servis
       etmiyor), bu yuzden notr renkte ve imlecsiz. */
    .gk-chip-atlas {
        display: inline-block; font-family: var(--mono); font-size: .76rem;
        color: var(--muted); border: 1px solid var(--line);
        background: rgba(148, 170, 200, .07);
        padding: .16rem .55rem; border-radius: var(--radius-sm);
    }
    .gk-status { display: inline-flex; align-items: center; gap: .4rem; font-size: .8rem; font-weight: 600; }
    .gk-status::before { content: ""; width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
    .gk-status.st-open { color: #FF9AA0; }
    .gk-status.st-ack { color: var(--accent); }
    .gk-status.st-closed { color: var(--faint); }
    .gk-count { font-family: var(--mono); font-variant-numeric: tabular-nums; color: var(--text); }
    /* Kaynak sistem sutunu (GKO-4): yalnizca cok urunlu kurulumda cikar */
    .gk-sys {
        font-size: .74rem; font-weight: 700; letter-spacing: .06em; white-space: nowrap;
        color: var(--accent); font-family: var(--mono);
    }
    .gk-time { color: var(--faint); font-size: .8rem; font-variant-numeric: tabular-nums; font-family: var(--mono); }

    /* Boş / hata durumları, dipnot */
    .gk-empty {
        position: relative; z-index: 1;
        border: 1px dashed var(--line-strong); border-radius: var(--radius-lg);
        padding: 3.2rem 1rem; text-align: center;
        color: var(--muted); background: var(--surface2);
    }
    .gk-empty-icon { color: var(--faint); margin-bottom: .9rem; }
    .gk-empty-title { color: var(--text); font-weight: 700; margin-bottom: .4rem; font-size: 1.02rem; }
    .gk-banner-err {
        position: relative; z-index: 1;
        display: flex; align-items: center; gap: .6rem;
        border: 1px solid rgba(255, 90, 97, .35); background: var(--crit-bg);
        color: #FFC1C4; padding: .8rem 1.1rem; border-radius: var(--radius-md);
        font-size: .86rem; margin-bottom: 1.1rem;
    }
    .gk-foot {
        position: relative; z-index: 1;
        display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: .5rem;
        color: var(--faint); font-size: .72rem; margin-top: 1.4rem;
        padding-top: 1rem; border-top: 1px solid var(--line);
        font-family: var(--mono);
    }
    </style>
    """,
    unsafe_allow_html=True,
)

# TEK SATIR olmasi bilincli: bu deger indent'li bir HTML blogunun ortasina
# f-string ile gomuluyor (bkz. asagidaki header st.markdown'i). Cok satirli
# ve satir basi (indentsiz) bir deger olsaydi, CommonMark bunu ayri/yeni bir
# blok sanip cevresindeki <div>'leri parcalar, geri kalani ise duz metin
# (indented code block) olarak basardi — tam olarak yasandi, olculdu.
GK_MARK_SVG = '<svg class="gk-mark" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="Göktürk"><path d="M20 2 L36 11 V29 L20 38 L4 29 V11 Z" stroke="#45C4D3" stroke-width="1.6" fill="rgba(69,196,211,.08)"/><path d="M20 9 L29 14 V26 L20 31 L11 26 V14 Z" stroke="#45C4D3" stroke-width="1.2" fill="none" opacity=".55"/><circle cx="20" cy="20" r="3.2" fill="#45C4D3"/></svg>'

ICON_BELL = '<svg class="gk-card-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>'
ICON_UNLOCK = '<svg class="gk-card-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 7.65-1.65"/></svg>'
ICON_ALERT = '<svg class="gk-card-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4"/><path d="M10.3 3.6 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.6a2 2 0 0 0-3.4 0Z"/><path d="M12 17h.01"/></svg>'
ICON_SHIELD = '<svg class="gk-card-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z"/></svg>'
ICON_RADAR = '<svg width="52" height="52" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" class="gk-empty-icon"><circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1"/><path d="M12 3v2M12 19v2M3 12h2M19 12h2"/></svg>'

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
      <div class="gk-brand">
        {GK_MARK_SVG}
        <div>
          <div class="gk-eyebrow">Deception Mesh · SOC Console</div>
          <div class="gk-title">GÖKTÜRK</div>
          <div class="gk-sub">Credential-canary alarm akışı · MITRE ATT&amp;CK korelasyonu</div>
        </div>
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
open_card_cls = " is-crit" if counts["open"] else ""
open_val_cls = " crit" if counts["open"] else ""
crit_card_cls = " is-crit" if counts["critical"] else ""
crit_val_cls = " crit" if counts["critical"] else ""
st.markdown(
    f"""
    <div class="gk-cards">
      <div class="gk-card">
        <div class="gk-card-top">
          <div class="gk-card-label">Toplam Alarm</div>{ICON_BELL}
        </div>
        <div class="gk-card-value">{counts["total"]}</div>
      </div>
      <div class="gk-card{open_card_cls}">
        <div class="gk-card-top">
          <div class="gk-card-label">Açık Alarm</div>{ICON_UNLOCK}
        </div>
        <div class="gk-card-value{open_val_cls}">{counts["open"]}</div>
      </div>
      <div class="gk-card{crit_card_cls}">
        <div class="gk-card-top">
          <div class="gk-card-label">Critical</div>{ICON_ALERT}
        </div>
        <div class="gk-card-value{crit_val_cls}">{counts["critical"]}</div>
      </div>
      <div class="gk-card is-accent">
        <div class="gk-card-top">
          <div class="gk-card-label">Aktif Tuzak</div>{ICON_SHIELD}
        </div>
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
              {ICON_RADAR}
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
        <div class="gk-table-head">
          <span class="gk-table-head-label">Aktif Alarmlar</span>
          <span class="gk-table-head-label">{len(alerts)} kayıt</span>
        </div>
        <div class="gk-table-scroll">
        <table class="gk-alerts">
          <thead><tr>
            <th>Önem</th>{"<th>Sistem</th>" if MULTI_SOURCE else ""}<th>Kaynak</th><th>Teknik</th><th>Durum</th>
            <th>Trip</th><th>İlk görülme</th><th>Son görülme</th>
          </tr></thead>
          <tbody>{rows}</tbody>
        </table>
        </div>
        </div>
        """,
        unsafe_allow_html=True,
    )

st.markdown(
    f'<div class="gk-foot">'
    f"<span>GÖKTÜRK Deception Mesh · deterministik korelasyon, sıfır-FP tezi</span>"
    f"<span>veriler {REFRESH_SECONDS} sn&apos;de bir yenilenir</span>"
    f"</div>",
    unsafe_allow_html=True,
)

time.sleep(REFRESH_SECONDS)
st.rerun()
