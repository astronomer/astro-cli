#!/bin/sh

# This script is used to generate a golang client for the Astro REST public API
# TODO: for now it gets the specs from the prod server, but eventually we will publish the specs in a public repo

CLIENTS_DIR=astro-client/generated
mkdir -p $CLIENTS_DIR

# Check if we need to install openapi-generator (https://github.com/OpenAPITools/openapi-generator)
# MacOS can simply use homebrew, but Linux must use either npm or a JAR (https://openapi-generator.tech/docs/installation/)
if ! command -v openapi-generator &> /dev/null; then
    echo "openapi-generator could not be found, installing it..."
    UNAME_S=$(shell uname -s)
    if [ ${UNAME_S} = Linux ]; then
        curl https://raw.githubusercontent.com/OpenAPITools/openapi-generator/master/bin/utils/openapi-generator-cli.sh > ${CLIENTS_DIR}/openapi-generator-cli
        chmod u+x ${CLIENTS_DIR}/openapi-generator-cli
        export PATH=$PATH:CLIENTS_DIR
    elif [ ${UNAME_S} = Darwin ]; then
        brew install openapi-generator
    else
        echo "unsupported OS ${UNAME_S}"
    fi
fi

# TODO only do this if the specs have changed

# Download the specs
TARGET_DIR=${CLIENTS_DIR}/golang/astropublicapi
mkdir -p ${TARGET_DIR}
curl -s https://api.astronomer.io/openapi/doc.json > ${TARGET_DIR}/openapi.json

# Generate the client
openapi-generator generate\
    -i ${TARGET_DIR}/openapi.json\
    -g go\
    --git-repo-id astro-client/generated/golang\
    --git-user-id astronomer\
    --additional-properties=packageName=astropublicapi,isGoSubmodule=true\
    -o ${TARGET_DIR} > /dev/null

go mod edit -replace=github.com/astronomer/astro-cli/astro-client/generated/golang/astropublicapi=./astro-client/generated/golang/astropublicapi
go mod tidy
