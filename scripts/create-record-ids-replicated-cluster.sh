#!/bin/bash

set -o errexit
set -o pipefail

# Check for required parameters
if [ $# -lt 2 ]; then
    echo "Usage: $0 <cluster_id> <cluster_type> [node_count]"
    echo "Example: $0 my-cluster REPLICASET 3"
    echo "Note: node_count defaults to 2 if not specified"
    exit 1
fi

if [ -z "$ATLAS_PUBLIC_KEY" ]; then
    echo "You must set the 'ATLAS_PUBLIC_KEY' env var when running this script"
    exit 1
fi

if [ -z "$ATLAS_PRIVATE_KEY" ]; then
    echo "You must set the 'ATLAS_PRIVATE_KEY' env var when running this script"
    exit 1
fi

CLUSTER_ID="$1"
CLUSTER_TYPE="$2"
ATLAS_BASE_URL="https://cloud-dev.mongodb.com/api/atlas/v2"
GROUP_ID="673e58327d4f1a7610a14faf"

NODE_COUNT="${3:-2}"
API_CREDENTIALS="$ATLAS_PUBLIC_KEY:$ATLAS_PRIVATE_KEY"

JSON_BODY=$(cat <<EOF
{
  "name": "$CLUSTER_ID",
  "clusterType": "$CLUSTER_TYPE",
  "replicationSpecs": [
    {
      "regionConfigs": [
        {
          "electableSpecs": {
            "instanceSize": "M30",
            "nodeCount": $NODE_COUNT
          },
          "priority": 7,
          "providerName": "AWS",
          "regionName": "US_EAST_1"
        }
      ]
    }
  ]
}
EOF
)

CLUSTER_URI="${ATLAS_BASE_URL}/groups/${GROUP_ID}/clusters"
echo $CLUSTER_URI
# Make the API call using the new Atlas v2 preview API
echo "Creating cluster '$CLUSTER_ID' with $NODE_COUNT node(s)..."
curl --user "$API_CREDENTIALS" \
     --digest \
     -X POST \
     -H "Content-Type: application/json" \
     -H "Accept: application/vnd.atlas.preview+json" \
     -d "$JSON_BODY" \
     $CLUSTER_URI
