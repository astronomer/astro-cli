#!/bin/bash

printf "Compiling bin data for:\n"

# remember root for later
ROOT=$(pwd)


# handle files compilation
printf " Files...\n"
cd templates
go-bindata -pkg templates files

cd $ROOT

printf "Compilation complete!\n"
