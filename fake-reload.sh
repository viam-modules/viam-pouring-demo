#!/usr/bin/env bash

set -eux

make -B bin/viam-pouring-demo
scp bin/viam-pouring-demo viam@mac-waiter.local:/Users/viam/viam-pouring-demo
viam module reload --part-id 7c2729a4-7bed-4699-a431-821abb26c468 --restart-only --name pouring-local-tmp

# note: this relies on the reload_path + reload_enabled fields here in config:
# {
#     "type": "registry",
#     "name": "viam_pouring-demo",
#     "module_id": "viam:pouring-demo",
#     "version": "latest",
#     "reload_path": "/Users/viam/pour-local.tar.gz",
#     "reload_enabled": true
# }
