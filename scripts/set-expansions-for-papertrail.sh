#!/bin/bash

set -e
set -x

tag=$(git describe --tags --always --dirty)

# This fails safe so that we use a non-production name by default and only use
# our real product name for actual tagged releases.
product_name="database-tools-dev"
if [[ "$tag" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    product_name="database-tools"
fi

cat <<EOT > expansions.yml
release_tag: $tag
product_name: $product_name
EOT
cat expansions.yml
