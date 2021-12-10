#!/bin/bash
# <UDF name="username" Label="Username" example="lbry" />
# <UDF name="password" Label="Password" example="ASDF123!@#" />
# <UDF name="private_ip_prefix" Label="PrivateIPPrefix" example="192" default="192" />
# <UDF name="snapshot_url" Label="SnapshotURL" default="https://snapshots.lbry.com/blockchain/lbcd/data-1052K.zip" />
# <UDF name="AWS_ACCESS_KEY_ID" Label="AccessKey" default="" />
# <UDF name="AWS_SECRET_ACCESS_KEY" Label="SecretKey" default="" />
# <UDF name="endpoint_url" Label="BlockStorageEndpoint" default="" />
# <UDF name="docker_compose_file_url" Label="ConfigURL" default="https://picardtek-linode-storage.us-east-1.linodeobjects.com/config/docker-compose-lbcd.yml" />

source <ssinclude StackScriptID=1>

# For debugging
exec > >(tee -i /var/log/stackscript.log) 2>&1
set -xeo pipefail

function user_add_sudo {
  USERNAME="$1"
  USERPASS="$2"
  if [ ! -n "$USERNAME" ] || [ ! -n "$USERPASS" ]; then
    echo "No new username and/or password entered"
    return 1;
  fi
  adduser "$USERNAME" --disabled-password --gecos ""
  echo "$USERNAME:$USERPASS" | chpasswd
  apt-get install -y sudo
  usermod -aG sudo "$USERNAME"
}

function download_snapshot {
  if [ -z "${AWS_ACCESS_KEY_ID}" ]; then
    wget "${SNAPSHOT_URL}"
  else
    echo "[default]
          access_key = ${AWS_ACCESS_KEY_ID}
          secret_key = ${AWS_SECRET_ACCESS_KEY}" > ~/.s3cfg
    if [ -z "${ENDPOINT_URL}" ]; then
      s4cmd --verbose get "${SNAPSHOT_URL}"
    else
      s4cmd --verbose get "${SNAPSHOT_URL}" --endpoint-url "${ENDPOINT_URL}"
    fi
  fi
}

function download_and_start {
  download_snapshot
  # get the snapshot data into place
  SNAPSHOT_FILE_NAME=$(echo "${SNAPSHOT_URL}" | rev | cut -d/ -f1 | rev)
  unzip "${SNAPSHOT_FILE_NAME}" -d ~/.lbcd/
  mv ~/.lbcd/"${SNAPSHOT_FILE_NAME}" ~/.lbcd/data
  rm "${SNAPSHOT_FILE_NAME}"
  # get our private ip
  PRIVATE_IP=$(ip addr | grep "${PRIVATE_IP_PREFIX}" | cut -d'/' -f1 | rev | cut -d" " -f 1 | rev)
  # download the compose-compose and put our private ip in the for RPC endpoint
  wget "${DOCKER_COMPOSE_FILE_URL}" -O - | sed 's/REPLACE_ME/'"${PRIVATE_IP}"'/g' > docker-compose.yml
  # Create our volume and start lbcd
  docker volume create --driver local \
      --opt type=none \
      --opt device=~/.lbcd\
      --opt o=bind lbcd
   docker-compose up -d
}
# add a non-root sudo user
user_add_sudo "${USERNAME}" "${PASSWORD}"
# Update and install dependencies
sudo apt-get update && sudo apt-get upgrade -y
sudo apt-get install -y unzip wget s4cmd
apt install -y apt-transport-https ca-certificates curl software-properties-common && \
        curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add - && \
        add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" && \
        apt install -y docker-ce docker-ce-cli containerd.io && \
        systemctl enable docker && sudo systemctl start docker && \
        curl -L "https://github.com/docker/compose/releases/download/1.24.1/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose && \
        chmod +x /usr/local/bin/docker-compose
# make sure we can use docker
usermod -aG docker $USERNAME
export -f download_and_start
export -f download_snapshot
su "${USERNAME}" -c 'bash -c "cd ~ && download_and_start"'