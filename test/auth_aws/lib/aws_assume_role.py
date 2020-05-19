#!/usr/bin/env python3
"""
Script for assuming an aws role.
"""

import argparse
import uuid
import logging

import boto3

LOGGER = logging.getLogger(__name__)

STS_DEFAULT_ROLE_NAME = "arn:aws:iam::557821124784:role/authtest_user_assume_role"

def _assume_role(role_name):
    sts_client = boto3.client("sts")

    print("RoleArn value: " + role_name)
    print("RoleSessionName value: " + str(uuid.uuid4()))
    response = sts_client.assume_role(RoleArn=role_name, RoleSessionName=str(uuid.uuid4()), DurationSeconds=900)

    creds = response["Credentials"]

    print("AccessKeyId: " + creds["AccessKeyId"])
    print("SecretAccessKey: " + creds["SecretAccessKey"])
    print("SessionToken: " + creds["SessionToken"])
    print("Expiration: " + creds["Expiration"])

#     print(f"""{{
#   "AccessKeyId" : "{creds["AccessKeyId"]}",
#   "SecretAccessKey" : "{creds["SecretAccessKey"]}",
#   "SessionToken" : "{creds["SessionToken"]}",
#   "Expiration" : "{str(creds["Expiration"])}"
# }}""")


def main() -> None:
    """Execute Main entry point."""

    parser = argparse.ArgumentParser(description='Assume Role frontend.')

    parser.add_argument('-v', "--verbose", action='store_true', help="Enable verbose logging")
    parser.add_argument('-d', "--debug", action='store_true', help="Enable debug logging")

    parser.add_argument('--role_name', type=str, default=STS_DEFAULT_ROLE_NAME, help="Role to assume")

    args = parser.parse_args()

    if args.debug:
        logging.basicConfig(level=logging.DEBUG)
    elif args.verbose:
        logging.basicConfig(level=logging.INFO)

    print("role_name: " + args.role_name)
    _assume_role(args.role_name)


if __name__ == "__main__":
    main()
