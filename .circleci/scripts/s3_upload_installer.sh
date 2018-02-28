#!/bin/bash

echo "Upload install.sh to s3://cli.astronomer.io/install.sh"
aws s3 cp --content-type "text/plain" ./install.sh s3://cli.astronomer.io/install.sh