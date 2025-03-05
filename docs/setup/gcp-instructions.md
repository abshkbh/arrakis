# Setup Instructions on GCP

## Setting Up a GCE VM with Nested Virtualization Support

- To create a Google Compute Engine (GCE) virtual machine with nested virtualization enabled, run the following command make sure to replace the $VM_NAME and $PROJECT with your own values.

    ```bash
    VM_NAME=<your-vm-name>
    PROJECT_ID=<your-project-id>
    SERVICE_ACCOUNT=<your-service-account>

    gcloud compute instances create ${VM_NAME} --project=${PROJECT_ID} --zone=us-west1-a --machine-type=n1-standard-1 --network-interface=network-tier=STANDARD,stack-type=IPV4_ONLY,subnet=default --maintenance-policy=MIGRATE --provisioning-model=STANDARD --service-account=${SERVICE_ACCOUNT} --scopes=https://www.googleapis.com/auth/devstorage.read_only,https://www.googleapis.com/auth/logging.write,https://www.googleapis.com/auth/monitoring.write,https://www.googleapis.com/auth/service.management.readonly,https://www.googleapis.com/auth/servicecontrol,https://www.googleapis.com/auth/trace.append --create-disk=auto-delete=yes,boot=yes,device-name=maverick-gcp-dev-vm3,image=projects/ubuntu-os-cloud/global/images/ubuntu-2204-jammy-v20250128,mode=rw,size=20,type=pd-standard --no-shielded-secure-boot --shielded-vtpm --shielded-integrity-monitoring --labels=goog-ec-src=vm_add-gcloud --reservation-affinity=any --enable-nested-virtualization
    ```

## Instructions to run on the GCE VM

- ssh into the VM.

    ```bash
    cd $HOME
    curl -LO "https://raw.githubusercontent.com/abshkbh/arrakis/btrfs-stateful-debugging/install-deps.sh"
    chmod +x install-deps.sh
    ./install-deps.sh
    source ~/.bashrc
    ```

- Build the project

    ```bash
    cd $HOME/projects/arrakis
    setup/install-images.py
    make clean && make all
    ```
- Verify it builds successfully

    ```bash
    ls out/
    ```
