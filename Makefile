OUT_DIR := out
API_CLIENT_DIR := out/gen/serverapi
API_CLIENT_GO_PACKAGE_NAME := serverapi
CHV_API_DIR := out/gen/chvapi
CHV_API_GO_PACKAGE_NAME := chvapi
RESTSERVER_BIN := ${OUT_DIR}/chv-restserver
CLIENT_BIN := ${OUT_DIR}/chv-client
GUESTINIT_BIN := ${OUT_DIR}/chv-guestinit
ROOTFSMAKER_BIN := ${OUT_DIR}/chv-rootfsmaker
CODESERVER_BIN := ${OUT_DIR}/chv-codeserver
CMDSERVER_BIN := ${OUT_DIR}/chv-cmdserver
GUESTROOTFS_BIN := ${OUT_DIR}/chv-guestrootfs-ext4.img

.PHONY: all clean serverapi chvapi restserver client guestinit rootfsmaker codeserver cmdserver guestrootfs guest

clean:
	rm -rf ${OUT_DIR}

all: serverapi chvapi restserver client guestinit rootfsmaker codeserver cmdserver guestrootfs guest

serverapi: ${OUT_DIR}/chv-serverapi.stamp
${OUT_DIR}/chv-serverapi.stamp: ./api/server-api.yaml
	mkdir -p ${API_CLIENT_DIR}
	openapi-generator-cli generate -i $< -g go -o ${API_CLIENT_DIR} --package-name ${API_CLIENT_GO_PACKAGE_NAME} \
	--git-user-id abshkbh \
	--git-repo-id chv-lambda/${API_CLIENT_DIR} \
    --additional-properties=withGoMod=false \
	--global-property models,supportingFiles,apis,apiTests=false
	rm -rf openapitools.json

chvapi: ${OUT_DIR}/chv-chvapi.stamp
${OUT_DIR}/chv-chvapi.stamp: api/chv-api.yaml
	mkdir -p ${API_CLIENT_DIR}
	openapi-generator-cli generate -i ./api/chv-api.yaml -g go -o ${CHV_API_DIR} --package-name ${CHV_API_GO_PACKAGE_NAME} \
	--git-user-id abshkbh \
	--git-repo-id chv-lambda/${CHV_API_DIR} \
    --additional-properties=withGoMod=false \
	--global-property models,supportingFiles,apis,apiTests=false
	rm -rf openapitools.json

restserver: serverapi chvapi
	mkdir -p ${OUT_DIR}
	go build -o ${RESTSERVER_BIN} ./cmd/restserver

client: serverapi
	mkdir -p ${OUT_DIR}
	go build -o ${CLIENT_BIN} ./cmd/client

# Build the guest init binary explicitly statically if "os" or "net" are used by
# using the CGO_ENABLED=0 flag.
guestinit:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${GUESTINIT_BIN} ./cmd/guestinit

rootfsmaker:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${ROOTFSMAKER_BIN} ./cmd/rootfsmaker

codeserver:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${CODESERVER_BIN} ./cmd/codeserver

cmdserver:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${CMDSERVER_BIN} ./cmd/cmdserver

guestrootfs: rootfsmaker guestinit
	mkdir -p ${OUT_DIR}
	sudo ${OUT_DIR}/chv-rootfsmaker create -o ${GUESTROOTFS_BIN} -d ./resources/scripts/rootfs/Dockerfile

guest: guestinit rootfsmaker codeserver cmdserver guestrootfs
