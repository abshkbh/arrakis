# Setup Instructions on GCP

## Setting Up a GCE VM with Nested Virtualization Support

- To create a Google Compute Engine (GCE) virtual machine with nested virtualization enabled, run the following command make sure to replace the $VM_NAME and $PROJECT with your own values.

    ```bash
    VM_NAME=<your-vm-name>
    PROJECT_ID=<your-project-id>
    SERVICE_ACCOUNT=<your-service-account>
    ZONE=<your-zone>

    gcloud compute instances create ${VM_NAME} --project=${PROJECT_ID} --zone=${ZONE} --machine-type=n1-standard-1 --network-interface=network-tier=STANDARD,stack-type=IPV4_ONLY,subnet=default --maintenance-policy=MIGRATE --provisioning-model=STANDARD --service-account=${SERVICE_ACCOUNT} --scopes=https://www.googleapis.com/auth/devstorage.read_only,https://www.googleapis.com/auth/logging.write,https://www.googleapis.com/auth/monitoring.write,https://www.googleapis.com/auth/service.management.readonly,https://www.googleapis.com/auth/servicecontrol,https://www.googleapis.com/auth/trace.append --create-disk=auto-delete=yes,boot=yes,device-name=maverick-gcp-dev-vm3,image=projects/ubuntu-os-cloud/global/images/ubuntu-2204-jammy-v20250128,mode=rw,size=20,type=pd-standard --no-shielded-secure-boot --shielded-vtpm --shielded-integrity-monitoring --labels=goog-ec-src=vm_add-gcloud --reservation-affinity=any --enable-nested-virtualization

    NETWORK_TAG=allow-ingress-ports
    FIREWALL_RULE=allow-ingress-ports-rule
    gcloud compute instances add-tags ${VM_NAME} --tags=${NETWORK_TAG} --zone=${ZONE}
    gcloud compute firewall-rules create ${FIREWALL_RULE} \
    --direction=INGRESS \
    --priority=1000 \
    --network=default \
    --action=ALLOW \
    --rules=tcp:3000-5000,tcp:7000 \
    --source-ranges=0.0.0.0/0 \
    --target-tags=${NETWORK_TAG} \
    --description="Allow TCP ingress on ports 3000-5000 and 7000 for VMs with the ${NETWORK_TAG} tag"
    ```

## Instructions to run on the GCE VM

- SSH into the VM.

    ```bash    
    # Use the setup script to install Arrakis
    cd $HOME
    curl -sSL "https://raw.githubusercontent.com/abshkbh/arrakis/main/setup/setup.sh" | bash
    ```

- Verify the installation

    ```bash
    cd $HOME/arrakis-prebuilt
    ls
    ```

- Run the Arrakis REST server

    ```bash
    sudo ./arrakis-restserver
    ```

- In another terminal, use the client to start sandboxes.

    ```bash
    cd ./arrakis-prebuilt
    ./arrakis-client
    ```

- Or use the Python SDK [py-arrakis](https://pypi.org/project/py-arrakis/) to start sandboxes.
