"""Göktürk Deception Mesh — canlı alarm feed'i (APP-9).

control-api'nin GET /api/v1/alerts'ini periyodik olarak çeker ve
severity/kaynak/ATT&CK tekniği/ilk-son görülme bilgisiyle gösterir.
AC (PROJECT PLAN.md APP-9): alarm oluşunca panelde ≤5 sn içinde belirir.
"""

import os
import time

import requests
import streamlit as st

from alerts_client import fetch_alerts, format_timestamp, severity_colors, summarize

CONTROL_API_URL = os.environ.get("CONTROL_API_URL", "http://localhost:8080")
REFRESH_SECONDS = int(os.environ.get("REFRESH_SECONDS", "3"))

st.set_page_config(page_title="Göktürk Deception Mesh", page_icon="🛰️", layout="wide")
st.title("🛰️ Göktürk Deception Mesh — Alarm Feed")
st.caption(f"Kaynak: {CONTROL_API_URL}/api/v1/alerts · {REFRESH_SECONDS} sn'de bir yenilenir")

try:
    alerts = fetch_alerts(CONTROL_API_URL)
except requests.RequestException as exc:
    st.error(f"control-api'ye ulaşılamadı: {exc}")
    alerts = []

if not alerts:
    st.info("Şu an açık/kayıtlı alarm yok.")
else:
    counts = summarize(alerts)
    col1, col2, col3 = st.columns(3)
    col1.metric("Toplam alarm", counts["total"])
    col2.metric("Açık alarm", counts["open"])
    col3.metric("Critical", counts["critical"])

    st.markdown(
        """
        <style>
        table.alerts-table { width: 100%; border-collapse: collapse; }
        table.alerts-table th, table.alerts-table td {
            text-align: left; padding: 8px 12px;
            border-bottom: 1px solid rgba(128,128,128,0.25);
            font-size: 0.9rem;
        }
        .severity-badge {
            padding: 2px 10px; border-radius: 999px; font-weight: 600; font-size: 0.85rem;
        }
        </style>
        """,
        unsafe_allow_html=True,
    )

    def render_row(a: dict) -> str:
        fg, bg = severity_colors(a.get("severity", ""))
        badge = (
            f'<span class="severity-badge" style="background:{bg};color:{fg};">'
            f'{a.get("severity", "")}</span>'
        )
        return (
            f"<tr><td>{badge}</td>"
            f"<td>{a.get('source', '')}</td>"
            f"<td>{a.get('technique') or '-'}</td>"
            f"<td>{a.get('status', '')}</td>"
            f"<td>{a.get('trip_count', 0)}</td>"
            f"<td>{format_timestamp(a['first_seen'])}</td>"
            f"<td>{format_timestamp(a['last_seen'])}</td></tr>"
        )

    rows = "".join(render_row(a) for a in alerts)
    st.markdown(
        f"""
        <table class="alerts-table">
          <thead><tr>
            <th>Severity</th><th>Kaynak</th><th>ATT&amp;CK</th><th>Durum</th>
            <th>Trip</th><th>İlk görülme</th><th>Son görülme</th>
          </tr></thead>
          <tbody>{rows}</tbody>
        </table>
        """,
        unsafe_allow_html=True,
    )

time.sleep(REFRESH_SECONDS)
st.rerun()
