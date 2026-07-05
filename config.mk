# Dynamically set default binary name to the directory name where the Makefile resides
BINARY_NAME ?= $(notdir $(CURDIR))
#DIR_NAME := $(notdir $(shell pwd))

# Define default variables (can be overridden from the command line)
GOOS ?= linux
GOARCH ?= amd64
OUTPUT_DIR ?= bin/$(GOOS)/$(GOARCH)
