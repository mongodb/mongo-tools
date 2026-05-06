#!/usr/bin/env python3
"""
Script for assuming an aws role.
"""

import argparse
import uuid
import logging
import os
import sys

import boto3

LOGGER = logging.getLogger(__name__)

STS_DEFAULT_ROLE_NAME = "arn:aws:iam::579766882180:role/mark.benvenuto"

def _assume_role(role_name):
    # Pass credentials explicitly to bypass boto3's credential chain (IMDS, files, inherited tokens).
    access_key_id = os.environ.get('AWS_ACCESS_KEY_ID')
    secret_access_key = os.environ.get('AWS_SECRET_ACCESS_KEY')

    if not access_key_id or not secret_access_key:
        LOGGER.error("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set in the environment")
        sys.exit(1)

    sts_client = boto3.client(
        "sts",
        aws_access_key_id=access_key_id,
        aws_secret_access_key=secret_access_key,
    )

    response = sts_client.assume_role(RoleArn=role_name, RoleSessionName=str(uuid.uuid4()), DurationSeconds=900)

    creds = response["Credentials"]

    print('{\n"AccessKeyId" : "' + creds['AccessKeyId'] +
     '",\n"SecretAccessKey" : "' + creds['SecretAccessKey'] +
      '",\n"SessionToken" : "' + creds['SessionToken'] +
       '",\n"Expiration" : "' + str(creds['Expiration']) + '"\n}')


def main():
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

    _assume_role(args.role_name)


if __name__ == "__main__":
    main()
