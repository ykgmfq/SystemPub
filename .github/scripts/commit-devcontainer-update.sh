#!/bin/bash
set -e
git config --global user.name "github-actions"
git config --global user.email "github-actions@github.com"
git checkout -b devcontainer-$1
git add .devcontainer/devcontainer.json
git commit -m "Update devcontainer to $1"
git push origin
