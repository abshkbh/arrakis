FROM ubuntu:20.04

# Avoid prompts from apt.
ENV DEBIAN_FRONTEND=noninteractive

ARG USERNAME=elara
ARG PASSWORD=elara0000
RUN useradd -m $USERNAME && \
    echo "$USERNAME:$PASSWORD" | chpasswd && \
    adduser $USERNAME sudo

# Needed to see journal logs.
RUN usermod -aG adm elara

# Set timezone.
ENV TZ=Etc/UTC
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# Update and install common utilities.
RUN apt-get update && apt-get upgrade -y && apt-get install -y \
    init \
    systemd \
    ncat \
    bash \
    curl \
    wget \
    vim \
    nano \
    git \
    htop \
    net-tools \
    iputils-ping \
    iproute2 \
    traceroute \
    dnsutils \
    tcpdump \
    netcat \
    ssh \
    sudo \
    man-db \
    less \
    procps \
    psmisc \
    lsof \
    rsync \
    tar \
    gzip \
    zip \
    unzip \
    ca-certificates \
    tzdata \
    tini \
    python3-venv \
    python3-pip \
    xvfb \
    xfce4 \
    xfce4-goodies \
    xrdp \
    zsh \
    tigervnc-standalone-server \
    btrfs-progs \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN echo "chv-starter-pack-vm" > /etc/hostname

RUN mkdir -p /var/lib/chv-guestinit

# To support sudo within the guest.
RUN chown root:root /usr/bin/sudo && chmod 4755 /usr/bin/sudo

# Add user binaries from the host into the guest rootfs in this section.
##############
COPY out/chv-guestinit /usr/local/bin/chv-guestinit
RUN chmod +x /usr/local/bin/chv-guestinit
COPY resources/chv-guestinit.service /usr/lib/systemd/system/chv-guestinit.service
RUN ln -s /usr/lib/systemd/system/chv-guestinit.service /etc/systemd/system/multi-user.target.wants/chv-guestinit.service

COPY out/chv-cmdserver /usr/local/bin/chv-cmdserver
RUN chmod +x /usr/local/bin/chv-cmdserver
COPY resources/chv-cmdserver.service /usr/lib/systemd/system/chv-cmdserver.service
RUN ln -s /usr/lib/systemd/system/chv-cmdserver.service /etc/systemd/system/multi-user.target.wants/chv-cmdserver.service

COPY resources/chv-vncserver.service /usr/lib/systemd/system/chv-vncserver.service
RUN ln -s /usr/lib/systemd/system/chv-vncserver.service /etc/systemd/system/multi-user.target.wants/chv-vncserver.service

# Optional binary that helps execute code.
#COPY out/chv-codeserver /opt/custom_scripts/chv-lambda-codeserver
##############

