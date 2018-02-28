#!/bin/bash

VERSION=$1
BUILD_DATE="`date '+%B %d, %Y @ %H:%M:%S'`"
CONTENT="package meta\n\nvar VERSION = \"$VERSION\"\nvar BUILD_DATE = \"$BUILD_DATE\""

mkdir -p meta

printf "$CONTENT" > ./meta/meta.go