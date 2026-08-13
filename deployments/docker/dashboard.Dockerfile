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

# Calisma zamani imajinda paket yoneticisi tutulmuyor: bagimliliklar build
# asamasinda kuruluyor, burada pip'in isi bitmis oluyor.
#
# Somut sebep: pip, vendor'ladigi bagimliliklari kendi icinde tasir
# (pip/_vendor/) ve bunlari bir CycloneDX SBOM'unda ilan eder
# (pip/_vendor/bom.cdx.json). Trivy bu SBOM'u okuyup msgpack 1.1.2 ve
# setuptools 70.3.0'i KURULU paket sayiyor ve OPS-4 kapisini kirmizi
# yakiyordu. Bu paketler dist-info'suz olduklari icin requirements.txt
# pinlemesiyle de `pip install --upgrade` ile de degistirilemezler —
# vendor'lanmis kaynak kodu pip'in kendi icindedir. Panel calisirken pip'i
# hic cagirmadigi icin dogru cozum bulguyu susturmak degil, o kodu imajdan
# gercekten cikarmak.
RUN python -m pip uninstall -y pip setuptools wheel 2>/dev/null; \
    rm -rf /usr/local/lib/python3.13/site-packages/pip* \
           /usr/local/lib/python3.13/site-packages/setuptools* \
           /usr/local/lib/python3.13/site-packages/pkg_resources \
           /usr/local/bin/pip*; \
    # Kaldirildigini burada dogrula: taban imaj yolu degistirirse yukaridaki
    # rm sessizce hicbir sey yapmaz ve imaj yeniden acikli hale doner.
    ! python -c "import pip" 2>/dev/null

WORKDIR /app
COPY --from=build --chown=dashboard:dashboard /root/.local /home/dashboard/.local
COPY --chown=dashboard:dashboard dashboard/app.py dashboard/alerts_client.py ./
COPY --chown=dashboard:dashboard dashboard/.streamlit ./.streamlit

ENV PATH=/home/dashboard/.local/bin:$PATH \
    PYTHONUNBUFFERED=1 \
    STREAMLIT_BROWSER_GATHER_USAGE_STATS=false

USER dashboard
EXPOSE 8501

# Streamlit'in dahili saglik ucu (/_stcore/health); ek bagimlilik gerekmez.
HEALTHCHECK --interval=5s --timeout=3s --retries=10 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8501/_stcore/health', timeout=2)" || exit 1

ENTRYPOINT ["streamlit", "run", "app.py", "--server.address=0.0.0.0", "--server.headless=true"]
