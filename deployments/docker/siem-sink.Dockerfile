# syntax=docker/dockerfile:1
# siem-sink (demo CEF alicisi, bkz. PROJECT PLAN.md APP-8 AC): socat'i BUILD
# ZAMANINDA (imaj online iken kurulurken) gomer. Onceki tanim bunu compose
# `command:`'inda calisma zamaninda `apk add` ile yapiyordu — bu, air-gapped
# bir hedefte (OPS-7) Alpine paket aynasina erisim gerektirdigi icin servisin
# hic baslamamasina yol acardi. Build context: repo koku.

FROM alpine:3.20
RUN apk add --no-cache socat

CMD ["sh", "-c", "echo 'siem-sink: udp/514 dinleniyor (CEF demo alicisi)' && exec socat -u UDP-RECVFROM:514,fork,reuseaddr -"]
