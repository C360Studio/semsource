#!/usr/bin/env bash
# Drop the per-store deployment authority (ID segment 2) from node handles and match ids.
jq '.results |= map(.answer |= (fromjson | (if .nodes then .nodes |= map(if (.handle|type)=="string" then .handle |= (split(".") | del(.[1]) | join(".")) else . end) else . end) | (if .matches then .matches |= map(if (.id|type)=="string" then .id |= (split(".") | del(.[1]) | join(".")) else . end) else . end) | tojson))' "$1"
