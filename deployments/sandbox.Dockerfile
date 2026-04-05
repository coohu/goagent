FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash curl wget git build-essential python3 python3-pip \
    nodejs npm jq ripgrep \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 1000 -s /bin/bash agent

WORKDIR /workspace
RUN chown agent:agent /workspace

USER agent

CMD ["/bin/bash"]
