#!/usr/bin/env bash

docker run --rm \
  -d --name newrelic-statsd \
  -h $(hostname) \
  -e NR_ACCOUNT_ID="" \
  -e NR_API_KEY="" \
  -e NR_EU_REGION=true \
  -p 8125:8125/udp \
  newrelic/nri-statsd:latest

