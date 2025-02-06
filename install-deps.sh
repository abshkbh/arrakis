#!/usr/bin/env bash
set -euo pipefail

# Update apt package list
echo "Updating apt package list..."
sudo apt update

# Install nvm using the provided install script
echo "Installing nvm..."
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash

# Load nvm into the current shell session
export NVM_DIR="$HOME/.nvm"
if [ -s "$NVM_DIR/nvm.sh" ]; then
  . "$NVM_DIR/nvm.sh"
else
  echo "nvm installation failed or nvm.sh not found."
  exit 1
fi

# Install Node.js using nvm
echo "Installing Node.js..."
nvm install node

# Install OpenAPI Generator CLI globally using npm
echo "Installing OpenAPI Generator CLI..."
npm install @openapitools/openapi-generator-cli -g

# Install Go programming language
# Ensure the go1.23.6.linux-amd64.tar.gz file is present in the current directory.
echo "Installing Go..."
sudo rm -rf /usr/local/go
curl -LO https://go.dev/dl/go1.23.6.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.6.linux-amd64.tar.gz
echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.bashrc

# Install default JDK without prompting for confirmation
echo "Installing default JDK..."
sudo apt install -y default-jdk

# Install Docker
echo "Installing Docker..."
echo "Removing old Docker packages if any..."
for pkg in docker.io docker-doc docker-compose docker-compose-v2 podman-docker containerd runc; do
  sudo apt-get remove -y "$pkg" || true
done

echo "Adding Docker's official GPG key and repository..."
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update

echo "Installing Docker CE and related packages..."
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

echo "Cloning project..."
mkdir -p $HOME/projects
cd $HOME/projects
git clone https://github.com/abshkbh/chv-starter-pack.git
cd $HOME

echo "Installation completed successfully."