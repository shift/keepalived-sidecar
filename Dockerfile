FROM cargo.caicloud.io/caicloud/keepalived:v1.2.19

RUN apt-get update && apt-get install -y --no-install-recommends \
  libssl1.0.0 \
  libnl-3-200 \
  libnl-route-3-200 \
  libnl-genl-3-200 \
  iptables \
  libnfnetlink0 \
  libiptcdata0 \
  libipset3 \
  libipset-dev \
  libsnmp30 \
  kmod \
  ca-certificates \
  iproute2 \
  ipvsadm \
  bash && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/*

COPY app /keepalived-sidecar
COPY keepalived.tmpl /
COPY keepalived.conf /etc/keepalived

CMD ["./keepalived-sidecar"]
