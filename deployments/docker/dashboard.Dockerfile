# syntax=docker/dockerfile:1
# Göktürk GÖKTÜRK Streamlit paneli (APP-9): control-api'nin GET /api/v1/alerts
# ucunu periyodik ceker, sifirdan yazilmistir (mevcut bir panel yoktu).
# Build context: repo koku (bkz. deployments/docker/docker-compose.yml)

FROM python:3.13-slim AS build
WORKDIR /app
COPY dashboard/requirements.txt .
RUN pip install --no-cache-dir --user -r requirements.txt

FROM python:3.13-slim
RUN useradd --create-home --uid 10001 dashboard
WORKDIR /app
COPY --from=build --chown=dashboard:dashboard /root/.local /home/dashboard/.local
COPY --chown=dashboard:dashboard dashboard/app.py dashboard/alerts_client.py ./

ENV PATH=/home/dashboard/.local/bin:$PATH \
    PYTHONUNBUFFERED=1 \
    STREAMLIT_BROWSER_GATHER_USAGE_STATS=false

USER dashboard
EXPOSE 8501

# Streamlit'in dahili saglik ucu (/_stcore/health); ek bagimlilik gerekmez.
HEALTHCHECK --interval=5s --timeout=3s --retries=10 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8501/_stcore/health', timeout=2)" || exit 1

ENTRYPOINT ["streamlit", "run", "app.py", "--server.address=0.0.0.0", "--server.headless=true"]
